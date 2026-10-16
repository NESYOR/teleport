/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 */

package livemap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport/api/types"
	apievents "github.com/gravitational/teleport/api/types/events"
)

// DefaultWatchKinds lists all Teleport Auth resource kinds that Consumer watches
// to dynamically build the live cluster topology and connection map.
var DefaultWatchKinds = []types.WatchKind{
	{Kind: types.KindAuthServer},
	{Kind: types.KindProxy},
	{Kind: types.KindNode},
	{Kind: types.KindDatabaseServer},
	{Kind: types.KindKubeServer},
	{Kind: types.KindAppServer},
	{Kind: types.KindTunnelConnection},
	{Kind: types.KindInstance},
}

// SessionTrackerGetter is an optional interface implemented by Auth clients
// that can list active user sessions.
type SessionTrackerGetter interface {
	GetActiveSessionTrackers(ctx context.Context) ([]types.SessionTracker, error)
}

// Consumer consumes Teleport Auth resource watch events (NewWatcher),
// active session trackers, and Audit Events (apievents.AuditEvent) and
// dynamically constructs a live cluster topology and data flow graph in a Hub.
type Consumer struct {
	hub        *Hub
	authNodeID string
}

// NewConsumer creates a new Auth event Consumer that populates the given Hub.
func NewConsumer(hub *Hub) *Consumer {
	if hub == nil {
		hub = DefaultHub
	}
	return &Consumer{
		hub:        hub,
		authNodeID: "auth-server",
	}
}

// SyncActiveSessions queries active SessionTrackers from Auth and maps each
// participant -> target resource session onto the topology graph.
func (c *Consumer) SyncActiveSessions(ctx context.Context, getter SessionTrackerGetter) error {
	trackers, err := getter.GetActiveSessionTrackers(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	for _, st := range trackers {
		c.handlePutEvent(st)
	}
	return nil
}

// WatchCluster opens a real-time NewWatcher stream against an Auth/Events client
// and continuously consumes OpPut and OpDelete events until ctx is canceled.
func (c *Consumer) WatchCluster(ctx context.Context, eventsClient types.Events) error {
	watcher, err := eventsClient.NewWatcher(ctx, types.Watch{
		Name:                "livemap-topology-consumer",
		Kinds:               DefaultWatchKinds,
		AllowPartialSuccess: true,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	defer watcher.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-watcher.Done():
			return trace.Wrap(watcher.Error())
		case ev := <-watcher.Events():
			c.ProcessResourceEvent(ev)
		}
	}
}

// ProcessResourceEvent translates a single Teleport Auth types.Event (OpPut/OpDelete)
// into topology node updates and live directed edge flow events.
func (c *Consumer) ProcessResourceEvent(ev types.Event) {
	switch ev.Type {
	case types.OpInit:
		return

	case types.OpDelete:
		if ev.Resource == nil {
			return
		}
		c.handleDeleteEvent(ev.Resource)

	case types.OpPut:
		if ev.Resource == nil {
			return
		}
		c.handlePutEvent(ev.Resource)
	}
}

func (c *Consumer) handlePutEvent(res types.Resource) {
	switch r := res.(type) {
	case types.Server:
		switch r.GetKind() {
		case types.KindAuthServer:
			id := "auth:" + r.GetName()
			c.authNodeID = id
			c.hub.UpsertNode(Node{
				ID:       id,
				Label:    firstNonEmpty(r.GetHostname(), r.GetName()),
				Kind:     "auth",
				Addr:     r.GetAddr(),
				Expiry:   r.Expiry(),
				LastSeen: time.Now(),
			})
			c.hub.RecordEvent(FlowEvent{
				Type:      EventHeartbeat,
				From:      id,
				FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
				FromKind:  "auth",
				FromAddr:  r.GetAddr(),
				To:        id,
				ToKind:    "auth",
				Summary:   "OpPut(AuthServer)",
				Details:   fmt.Sprintf("hostname=%s addr=%s version=%s", r.GetHostname(), r.GetAddr(), r.GetTeleportVersion()),
			})

		case types.KindProxy:
			id := "proxy:" + r.GetName()
			c.hub.RecordEvent(FlowEvent{
				Type:      EventHeartbeat,
				From:      id,
				FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
				FromKind:  "proxy",
				FromAddr:  firstNonEmpty(r.GetPublicAddr(), r.GetAddr()),
				To:        c.authNodeID,
				ToKind:    "auth",
				Summary:   "OpPut(Proxy Heartbeat)",
				Details:   fmt.Sprintf("hostname=%s public_addr=%s peer_addr=%s", r.GetHostname(), r.GetPublicAddr(), r.GetPeerAddr()),
			})

		case types.KindNode:
			id := "node:" + r.GetName()
			c.hub.RecordEvent(FlowEvent{
				Type:      EventHeartbeat,
				From:      id,
				FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
				FromKind:  "node",
				FromAddr:  r.GetAddr(),
				To:        c.authNodeID,
				ToKind:    "auth",
				Summary:   "OpPut(Node Heartbeat)",
				Details:   fmt.Sprintf("hostname=%s addr=%s tunnel=%v ttl=%s", r.GetHostname(), r.GetAddr(), r.GetUseTunnel(), time.Until(r.Expiry()).Round(time.Second)),
			})
		}

	case types.DatabaseServer:
		id := "db:" + r.GetDatabase().GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: r.GetDatabase().GetName(),
			FromKind:  "db",
			FromAddr:  r.GetDatabase().GetURI(),
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(DatabaseServer Heartbeat)",
			Details:   fmt.Sprintf("host_id=%s protocol=%s uri=%s", r.GetHostID(), r.GetDatabase().GetProtocol(), r.GetDatabase().GetURI()),
		})

	case types.KubeServer:
		clusterName := r.GetName()
		if r.GetCluster() != nil {
			clusterName = r.GetCluster().GetName()
		}
		id := "kube:" + clusterName
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: clusterName,
			FromKind:  "kube",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(KubeServer Heartbeat)",
			Details:   fmt.Sprintf("host_id=%s kube_cluster=%s", r.GetHostID(), clusterName),
		})

	case types.AppServer:
		appName := r.GetName()
		uri := ""
		if r.GetApp() != nil {
			appName = r.GetApp().GetName()
			uri = r.GetApp().GetURI()
		}
		id := "app:" + appName
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: appName,
			FromKind:  "app",
			FromAddr:  uri,
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(AppServer Heartbeat)",
			Details:   fmt.Sprintf("host_id=%s app=%s uri=%s", r.GetHostID(), appName, uri),
		})

	case types.TunnelConnection:
		fromKind := string(r.GetType())
		fromID := fmt.Sprintf("%s:%s", fromKind, r.GetName())
		toProxy := "proxy:" + r.GetProxyName()
		c.hub.RecordEvent(FlowEvent{
			Type:     EventTunnelHB,
			From:     fromID,
			FromKind: fromKind,
			To:       toProxy,
			ToKind:   "proxy",
			Summary:  fmt.Sprintf("TunnelConnection(%s)", r.GetType()),
			Details:  fmt.Sprintf("cluster=%s proxy=%s last_hb=%s", r.GetClusterName(), r.GetProxyName(), r.GetLastHeartbeat().Format(time.RFC3339)),
		})

	case types.SessionTracker:
		targetID, targetKind := resolveSessionTarget(r)
		for _, p := range r.GetParticipants() {
			if p.User == "" {
				continue
			}
			userID := "user:" + p.User
			c.hub.RecordEvent(FlowEvent{
				Type:      EventLiveSession,
				From:      userID,
				FromLabel: p.User,
				FromKind:  "user",
				To:        targetID,
				ToKind:    targetKind,
				Summary:   fmt.Sprintf("Session(%s) [%s]", r.GetSessionKind(), r.GetState()),
				Details:   fmt.Sprintf("session_id=%s login=%s mode=%s", r.GetSessionID(), r.GetLogin(), p.Mode),
			})
		}
	}
}

func (c *Consumer) handleDeleteEvent(res types.Resource) {
	kind := res.GetKind()
	name := res.GetName()

	switch kind {
	case types.KindNode:
		id := "node:" + name
		c.hub.RemoveNode(id)
		c.hub.RecordEvent(FlowEvent{
			Type:     EventNodeRemoved,
			From:     id,
			FromKind: "node",
			To:       c.authNodeID,
			ToKind:   "auth",
			Summary:  "OpDelete(Node Expired/Disconnected)",
			Details:  fmt.Sprintf("node=%s", name),
		})
	case types.KindProxy:
		id := "proxy:" + name
		c.hub.RemoveNode(id)
	case types.KindSessionTracker:
		c.hub.RecordEvent(FlowEvent{
			Type:    EventNodeRemoved,
			From:    c.authNodeID,
			To:      c.authNodeID,
			Summary: "OpDelete(Session Ended)",
			Details: fmt.Sprintf("session_id=%s", name),
		})
	}
}

// EmitAuditEvent implements apievents.Emitter so Consumer can be hooked into
// an Auth Server's audit pipeline or fed events from SearchEvents.
func (c *Consumer) EmitAuditEvent(_ context.Context, ev apievents.AuditEvent) error {
	c.ProcessAuditEvent(ev)
	return nil
}

// ProcessAuditEvent translates a Teleport AuditEvent (login, cert.create, session.start,
// db.session.query, kube.request, etc.) into a live flow edge on the topology map.
func (c *Consumer) ProcessAuditEvent(ev apievents.AuditEvent) {
	if ev == nil {
		return
	}
	user := "system"
	if getter, ok := ev.(interface{ GetUser() string }); ok && getter.GetUser() != "" {
		user = "user:" + getter.GetUser()
	}

	target := c.authNodeID
	targetKind := "auth"
	if sGetter, ok := ev.(interface{ GetServerID() string }); ok && sGetter.GetServerID() != "" {
		target = "node:" + sGetter.GetServerID()
		targetKind = "node"
	} else if dbGetter, ok := ev.(interface{ GetDatabaseService() string }); ok && dbGetter.GetDatabaseService() != "" {
		target = "db:" + dbGetter.GetDatabaseService()
		targetKind = "db"
	} else if kubeGetter, ok := ev.(interface{ GetKubernetesCluster() string }); ok && kubeGetter.GetKubernetesCluster() != "" {
		target = "kube:" + kubeGetter.GetKubernetesCluster()
		targetKind = "kube"
	}

	c.hub.RecordEvent(FlowEvent{
		Timestamp: ev.GetTime(),
		Type:      EventAudit,
		From:      user,
		FromKind:  "user",
		To:        target,
		ToKind:    targetKind,
		Summary:   fmt.Sprintf("Audit(%s)", ev.GetType()),
		Details:   fmt.Sprintf("code=%s cluster=%s", ev.GetCode(), ev.GetClusterName()),
	})
}

func resolveSessionTarget(st types.SessionTracker) (string, string) {
	switch {
	case st.GetKubeCluster() != "":
		return "kube:" + st.GetKubeCluster(), "kube"
	case st.GetDatabaseName() != "":
		return "db:" + st.GetDatabaseName(), "db"
	case st.GetAppName() != "":
		return "app:" + st.GetAppName(), "app"
	case st.GetHostID() != "":
		// Strip cluster prefix if present ("cluster/host-uuid")
		parts := strings.Split(st.GetHostID(), "/")
		return "node:" + parts[len(parts)-1], "node"
	case st.GetHostname() != "":
		return "node:" + st.GetHostname(), "node"
	default:
		return "auth-server", "auth"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
