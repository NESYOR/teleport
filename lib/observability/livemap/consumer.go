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

	scopesv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/scopes/v1"
	"github.com/gravitational/teleport/api/types"
	apievents "github.com/gravitational/teleport/api/types/events"
	scopedaccess "github.com/gravitational/teleport/lib/scopes/access"
)

var allScopes = types.ScopeFilterFromProto(scopesv1.Filter_builder{Mode: scopesv1.Mode_MODE_ALL}.Build())

// ProxyWatchKinds lists all resource kinds watched by a Teleport Proxy Service
// (matching cache.ForProxy in lib/cache/cache.go).
var ProxyWatchKinds = []types.WatchKind{
	{Kind: types.KindCertAuthority, LoadSecrets: false},
	{Kind: types.KindClusterName},
	{Kind: types.KindClusterAuditConfig},
	{Kind: types.KindClusterNetworkingConfig},
	{Kind: types.KindClusterAuthPreference},
	{Kind: types.KindSessionRecordingConfig},
	{Kind: types.KindUIConfig},
	{Kind: types.KindUser},
	{Kind: types.KindRole},
	{Kind: types.KindNode, ScopeFilter: allScopes},
	{Kind: types.KindProxy},
	{Kind: types.KindAuthServer},
	{Kind: types.KindReverseTunnel},
	{Kind: types.KindTunnelConnection},
	{Kind: types.KindAppServer, ScopeFilter: allScopes},
	{Kind: types.KindApp, ScopeFilter: allScopes},
	{Kind: types.KindWebSession, SubKind: types.KindSnowflakeSession},
	{Kind: types.KindWebSession, SubKind: types.KindAppSession},
	{Kind: types.KindWebSession, SubKind: types.KindWebSession},
	{Kind: types.KindWebToken},
	{Kind: types.KindRemoteCluster},
	{Kind: types.KindDatabaseServer, ScopeFilter: allScopes},
	{Kind: types.KindDatabaseService},
	{Kind: types.KindDatabase},
	{Kind: types.KindWindowsDesktopService},
	{Kind: types.KindWindowsDesktop},
	{Kind: types.KindDynamicWindowsDesktop},
	{Kind: types.KindLinuxDesktop},
	{Kind: types.KindKubeServer, ScopeFilter: allScopes},
	{Kind: types.KindKubernetesCluster, ScopeFilter: allScopes},
	{Kind: types.KindSAMLIdPServiceProvider},
	{Kind: types.KindUserGroup},
	{Kind: types.KindIntegration},
	{Kind: types.KindAuditQuery},
	{Kind: types.KindSecurityReport},
	{Kind: types.KindSecurityReportState},
	{Kind: types.KindKubeWaitingContainer},
	{Kind: types.KindAutoUpdateConfig},
	{Kind: types.KindAutoUpdateVersion},
	{Kind: types.KindAutoUpdateAgentRollout},
	{Kind: types.KindUserTask},
	{Kind: types.KindGitServer},
	{Kind: types.KindRelayServer},
	{Kind: types.KindHealthCheckConfig},
}

// DefaultWatchKinds lists the complete union of all Teleport Auth and Proxy
// resource kinds (matching cache.ForAuth + cache.ForProxy + inventory/device kinds)
// so Consumer captures every cluster heartbeat, tunnel, session, access request,
// lock, CA rotation, RBAC change, and configuration update in real time.
var DefaultWatchKinds = []types.WatchKind{
	// 1. Presence, Heartbeats & Infrastructure (Proxy + Auth)
	{Kind: types.KindInstance},
	{Kind: types.KindAuthServer},
	{Kind: types.KindProxy},
	{Kind: types.KindRelayServer},
	{Kind: types.KindNode, ScopeFilter: allScopes},
	{Kind: types.KindDatabaseServer, ScopeFilter: allScopes},
	{Kind: types.KindDatabaseService},
	{Kind: types.KindDatabase},
	{Kind: types.KindDatabaseObject},
	{Kind: types.KindKubeServer, ScopeFilter: allScopes},
	{Kind: types.KindKubernetesCluster, ScopeFilter: allScopes},
	{Kind: types.KindAppServer, ScopeFilter: allScopes},
	{Kind: types.KindApp, ScopeFilter: allScopes},
	{Kind: types.KindWindowsDesktopService},
	{Kind: types.KindWindowsDesktop},
	{Kind: types.KindDynamicWindowsDesktop},
	{Kind: types.KindLinuxDesktop},
	{Kind: types.KindGitServer},
	{Kind: types.KindBotInstance, ScopeFilter: allScopes},

	// 2. Reverse Tunnels & Multi-Cluster Trust (Proxy + Auth)
	{Kind: types.KindTunnelConnection},
	{Kind: types.KindReverseTunnel},
	{Kind: types.KindRemoteCluster},

	// 3. Live Sessions, Web Sessions & Interactive Handshakes (Proxy + Auth)
	{Kind: types.KindWebSession, SubKind: types.KindWebSession},
	{Kind: types.KindWebSession, SubKind: types.KindAppSession},
	{Kind: types.KindWebSession, SubKind: types.KindSnowflakeSession},
	{Kind: types.KindWebToken},
	{Kind: types.KindHeadlessAuthentication},
	{Kind: types.KindValidatedMFAChallenge},
	{Kind: types.KindKubeWaitingContainer},
	{Kind: types.KindBeam},
	{Kind: types.KindBeamsConfig},

	// 4. Zero-Trust Access Requests, Locks & Governance (Auth)
	{Kind: types.KindAccessRequest},
	{Kind: types.KindLock},
	{Kind: types.KindAccessList},
	{Kind: types.KindAccessListMember},
	{Kind: types.KindAccessListReview},
	{Kind: types.KindAccessMonitoringRule},
	{Kind: types.KindCrownJewel},
	{Kind: types.KindAccessGraphSettings},

	// 5. Certificate Authorities, Trust & Identity (Proxy + Auth)
	{Kind: types.KindCertAuthority, LoadSecrets: false},
	{Kind: types.KindCertAuthorityOverride},
	{Kind: types.KindPendingCSRRequest},
	{Kind: types.KindSPIFFEFederation},
	{Kind: types.KindSAMLIdPServiceProvider},
	{Kind: types.KindWorkloadIdentity, ScopeFilter: allScopes},
	{Kind: types.KindRecordingEncryption},

	// 6. RBAC, Users, Roles, Tokens & Devices (Proxy + Auth)
	{Kind: types.KindUser},
	{Kind: types.KindUserLoginState},
	{Kind: types.KindRole},
	{Kind: scopedaccess.KindScopedRole},
	{Kind: scopedaccess.KindScopedRoleAssignment},
	{Kind: types.KindUserGroup},
	{Kind: types.KindToken},
	{Kind: types.KindStaticTokens},
	{Kind: types.KindStaticScopedTokens},
	{Kind: types.KindStaticHostUser},
	{Kind: types.KindDevice},
	{Kind: types.KindOktaImportRule},
	{Kind: types.KindOktaAssignment},
	{Kind: types.KindProvisioningPrincipalState},
	{Kind: types.KindIdentityCenterAccount},
	{Kind: types.KindIdentityCenterPrincipalAssignment},
	{Kind: types.KindIdentityCenterAccountAssignment},

	// 7. Cluster Configuration, Auto-Update Rollouts, Discovery & Plugins (Proxy + Auth)
	{Kind: types.KindClusterName},
	{Kind: types.KindClusterAuthPreference},
	{Kind: types.KindClusterNetworkingConfig},
	{Kind: types.KindSessionRecordingConfig},
	{Kind: types.KindClusterAuditConfig},
	{Kind: types.KindUIConfig},
	{Kind: types.KindNetworkRestrictions},
	{Kind: types.KindInstaller},
	{Kind: types.KindDiscoveryConfig},
	{Kind: types.KindIntegration},
	{Kind: types.KindPlugin},
	{Kind: types.KindPluginStaticCredentials},
	{Kind: types.KindAutoUpdateConfig},
	{Kind: types.KindAutoUpdateVersion},
	{Kind: types.KindAutoUpdateAgentRollout},
	{Kind: types.KindAutoUpdateAgentReport},
	{Kind: types.KindAutoUpdateBotInstanceReport},
	{Kind: types.KindHealthCheckConfig},
	{Kind: types.KindAuditQuery},
	{Kind: types.KindSecurityReport},
	{Kind: types.KindSecurityReportState},
	{Kind: types.KindNotification},
	{Kind: types.KindGlobalNotification},
	{Kind: types.KindUserTask},
	{Kind: types.KindInferenceModel},
	{Kind: types.KindInferencePolicy},
	{Kind: types.KindInferenceSecret},
	{Kind: types.KindClassifier},
	{Kind: types.KindRetrievalModel},
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
// subscribed to all Auth and Proxy watch kinds (DefaultWatchKinds) and
// continuously consumes OpPut and OpDelete events until ctx is canceled.
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
	// =========================================================================
	// 1. INFRASTRUCTURE PRESENCE & HEARTBEATS (Proxy + Auth)
	// =========================================================================
	case types.Instance:
		services := make([]string, 0, len(r.GetServices()))
		primaryKind := "node"
		for _, svc := range r.GetServices() {
			services = append(services, strings.ToLower(string(svc)))
			switch svc {
			case types.RoleAuth:
				primaryKind = "auth"
			case types.RoleProxy:
				if primaryKind != "auth" {
					primaryKind = "proxy"
				}
			case types.RoleDatabase:
				primaryKind = "db"
			case types.RoleKube:
				primaryKind = "kube"
			case types.RoleApp:
				primaryKind = "app"
			}
		}
		id := primaryKind + ":" + r.GetName()
		targetAuth := c.authNodeID
		if r.GetAuthID() != "" {
			targetAuth = "auth:" + r.GetAuthID()
		}
		c.hub.RecordEvent(FlowEvent{
			Type:      EventInstanceHB,
			From:      id,
			FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
			FromKind:  primaryKind,
			To:        targetAuth,
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(Instance [%s])", strings.Join(services, ",")),
			Details:   fmt.Sprintf("server_id=%s hostname=%s version=%s services=%v", r.GetName(), r.GetHostname(), r.GetTeleportVersion(), services),
		})

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

		default:
			id := r.GetKind() + ":" + r.GetName()
			c.hub.RecordEvent(FlowEvent{
				Type:      EventHeartbeat,
				From:      id,
				FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
				FromKind:  "node",
				FromAddr:  r.GetAddr(),
				To:        c.authNodeID,
				ToKind:    "auth",
				Summary:   fmt.Sprintf("OpPut(%s Heartbeat)", r.GetKind()),
				Details:   fmt.Sprintf("hostname=%s addr=%s", r.GetHostname(), r.GetAddr()),
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

	case types.Database:
		id := "db:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: r.GetName(),
			FromKind:  "db",
			FromAddr:  r.GetURI(),
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(Database Resource)",
			Details:   fmt.Sprintf("protocol=%s uri=%s", r.GetProtocol(), r.GetURI()),
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

	case types.KubeCluster:
		id := "kube:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: r.GetName(),
			FromKind:  "kube",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(KubernetesCluster)",
			Details:   fmt.Sprintf("name=%s", r.GetName()),
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

	case types.Application:
		id := "app:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: r.GetName(),
			FromKind:  "app",
			FromAddr:  r.GetURI(),
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(App Resource)",
			Details:   fmt.Sprintf("app=%s uri=%s public_addr=%s", r.GetName(), r.GetURI(), r.GetPublicAddr()),
		})

	case types.WindowsDesktopService:
		id := "desktop-svc:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: firstNonEmpty(r.GetHostname(), r.GetName()),
			FromKind:  "node",
			FromAddr:  r.GetAddr(),
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(WindowsDesktopService Heartbeat)",
			Details:   fmt.Sprintf("hostname=%s addr=%s", r.GetHostname(), r.GetAddr()),
		})

	case types.WindowsDesktop:
		id := "desktop:" + r.GetName()
		hostID := c.authNodeID
		if r.GetHostID() != "" {
			hostID = "desktop-svc:" + r.GetHostID()
		}
		c.hub.RecordEvent(FlowEvent{
			Type:      EventHeartbeat,
			From:      id,
			FromLabel: r.GetName(),
			FromKind:  "node",
			FromAddr:  r.GetAddr(),
			To:        hostID,
			ToKind:    "node",
			Summary:   "OpPut(WindowsDesktop)",
			Details:   fmt.Sprintf("desktop=%s addr=%s domain=%s", r.GetName(), r.GetAddr(), r.GetDomain()),
		})

	// =========================================================================
	// 2. REVERSE TUNNELS & MULTI-CLUSTER TRUST (Proxy + Auth)
	// =========================================================================
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
			Summary:  fmt.Sprintf("OpPut(TunnelConnection [%s])", r.GetType()),
			Details:  fmt.Sprintf("cluster=%s proxy=%s last_hb=%s", r.GetClusterName(), r.GetProxyName(), r.GetLastHeartbeat().Format(time.RFC3339)),
		})

	case types.ReverseTunnel:
		fromCluster := "cluster:" + r.GetClusterName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventTunnelHB,
			From:      fromCluster,
			FromLabel: r.GetClusterName(),
			FromKind:  "proxy",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(ReverseTunnel [%s])", r.GetType()),
			Details:   fmt.Sprintf("cluster=%s dial_addrs=%v", r.GetClusterName(), r.GetDialAddrs()),
		})

	case types.RemoteCluster:
		fromCluster := "cluster:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventTunnelHB,
			From:      fromCluster,
			FromLabel: r.GetName(),
			FromKind:  "proxy",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(RemoteCluster [%s])", r.GetConnectionStatus()),
			Details:   fmt.Sprintf("cluster=%s status=%s last_hb=%s", r.GetName(), r.GetConnectionStatus(), r.GetLastHeartbeat().Format(time.RFC3339)),
		})

	// =========================================================================
	// 3. LIVE SESSIONS, WEB SESSIONS & HANDSHAKES (Proxy + Auth)
	// =========================================================================
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

	case types.WebSession:
		userID := "user:" + firstNonEmpty(r.GetUser(), "anonymous")
		subKind := firstNonEmpty(r.GetSubKind(), types.KindWebSession)
		c.hub.RecordEvent(FlowEvent{
			Type:      EventLiveSession,
			From:      userID,
			FromLabel: r.GetUser(),
			FromKind:  "user",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(%s)", subKind),
			Details:   fmt.Sprintf("user=%s session=%s expires=%s", r.GetUser(), r.GetShortName(), r.GetBearerTokenExpiryTime().Format(time.RFC3339)),
		})

	case types.WebToken:
		userID := "user:" + firstNonEmpty(r.GetUser(), "anonymous")
		c.hub.RecordEvent(FlowEvent{
			Type:      EventLiveSession,
			From:      userID,
			FromLabel: r.GetUser(),
			FromKind:  "user",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   "OpPut(WebToken)",
			Details:   fmt.Sprintf("user=%s subkind=%s", r.GetUser(), r.GetSubKind()),
		})

	// =========================================================================
	// 4. ZERO-TRUST ACCESS REQUESTS & SESSION LOCKS (Auth)
	// =========================================================================
	case types.AccessRequest:
		userID := "user:" + r.GetUser()
		allowed := r.GetState().IsApproved() || r.GetState().IsPending()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventAccessRequest,
			From:      userID,
			FromLabel: r.GetUser(),
			FromKind:  "user",
			To:        c.authNodeID,
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(AccessRequest [%s])", r.GetState()),
			Details:   fmt.Sprintf("id=%s user=%s roles=%v reason=%q", r.GetName(), r.GetUser(), r.GetRoles(), r.GetRequestReason()),
			Allowed:   &allowed,
		})

	case types.Lock:
		target := r.Target()
		targetID := c.authNodeID
		targetKind := "auth"
		switch {
		case target.User != "":
			targetID = "user:" + target.User
			targetKind = "user"
		case target.ServerID != "":
			targetID = "node:" + target.ServerID
			targetKind = "node"
		case target.Role != "":
			targetID = "role:" + target.Role
			targetKind = "user"
		}
		denied := false
		c.hub.RecordEvent(FlowEvent{
			Type:    EventLock,
			From:    c.authNodeID,
			To:      targetID,
			ToKind:  targetKind,
			Summary: fmt.Sprintf("OpPut(Lock -> %v)", target),
			Details: fmt.Sprintf("lock=%s message=%q created_by=%s", r.GetName(), r.Message(), r.CreatedBy()),
			Allowed: &denied,
		})

	// =========================================================================
	// 5. CERTIFICATE AUTHORITIES & RBAC RESOURCES (Proxy + Auth)
	// =========================================================================
	case types.CertAuthority:
		caNode := fmt.Sprintf("ca:%s:%s", r.GetType(), r.GetClusterName())
		c.hub.RecordEvent(FlowEvent{
			Type:      EventCARotation,
			From:      c.authNodeID,
			To:        caNode,
			ToLabel:   fmt.Sprintf("CA(%s)", r.GetType()),
			ToKind:    "auth",
			Summary:   fmt.Sprintf("OpPut(CertAuthority [%s])", r.GetType()),
			Details:   fmt.Sprintf("cluster=%s type=%s rotation_phase=%s", r.GetClusterName(), r.GetType(), r.GetRotation().Phase),
		})

	case types.User:
		userID := "user:" + r.GetName()
		c.hub.RecordEvent(FlowEvent{
			Type:      EventRBACChange,
			From:      c.authNodeID,
			To:        userID,
			ToLabel:   r.GetName(),
			ToKind:    "user",
			Summary:   "OpPut(User)",
			Details:   fmt.Sprintf("user=%s roles=%v", r.GetName(), r.GetRoles()),
		})

	case types.Role:
		c.hub.RecordEvent(FlowEvent{
			Type:    EventRBACChange,
			From:    c.authNodeID,
			To:      c.authNodeID,
			Summary: fmt.Sprintf("OpPut(Role [%s])", r.GetName()),
			Details: fmt.Sprintf("role=%s", r.GetName()),
		})

	case types.ProvisionToken:
		c.hub.RecordEvent(FlowEvent{
			Type:    EventRBACChange,
			From:    c.authNodeID,
			To:      c.authNodeID,
			Summary: fmt.Sprintf("OpPut(Token [%v])", r.GetRoles()),
			Details: fmt.Sprintf("roles=%v join_method=%s", r.GetRoles(), r.GetJoinMethod()),
		})

	// =========================================================================
	// 6. GENERIC FALLBACK FOR ALL OTHER WATCHED AUTH & PROXY RESOURCES
	//    (ClusterConfig, AutoUpdate, AccessList, DiscoveryConfig, KubeWaitingContainer, etc.)
	// =========================================================================
	default:
		kind := res.GetKind()
		name := res.GetName()
		evType := classifyResourceEventType(kind)
		c.hub.RecordEvent(FlowEvent{
			Type:    evType,
			From:    c.authNodeID,
			To:      c.authNodeID,
			Summary: fmt.Sprintf("OpPut(%s)", kind),
			Details: fmt.Sprintf("kind=%s subkind=%s name=%s", kind, res.GetSubKind(), name),
		})
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
		c.hub.RecordEvent(FlowEvent{
			Type:     EventNodeRemoved,
			From:     id,
			FromKind: "proxy",
			To:       c.authNodeID,
			ToKind:   "auth",
			Summary:  "OpDelete(Proxy Expired/Disconnected)",
			Details:  fmt.Sprintf("proxy=%s", name),
		})

	case types.KindDatabaseServer, types.KindDatabase:
		id := "db:" + name
		c.hub.RemoveNode(id)
		c.hub.RecordEvent(FlowEvent{
			Type:     EventNodeRemoved,
			From:     id,
			FromKind: "db",
			To:       c.authNodeID,
			ToKind:   "auth",
			Summary:  fmt.Sprintf("OpDelete(%s Expired)", kind),
			Details:  fmt.Sprintf("name=%s", name),
		})

	case types.KindKubeServer, types.KindKubernetesCluster:
		id := "kube:" + name
		c.hub.RemoveNode(id)
		c.hub.RecordEvent(FlowEvent{
			Type:     EventNodeRemoved,
			From:     id,
			FromKind: "kube",
			To:       c.authNodeID,
			ToKind:   "auth",
			Summary:  fmt.Sprintf("OpDelete(%s Expired)", kind),
			Details:  fmt.Sprintf("name=%s", name),
		})

	case types.KindAppServer, types.KindApp:
		id := "app:" + name
		c.hub.RemoveNode(id)
		c.hub.RecordEvent(FlowEvent{
			Type:     EventNodeRemoved,
			From:     id,
			FromKind: "app",
			To:       c.authNodeID,
			ToKind:   "auth",
			Summary:  fmt.Sprintf("OpDelete(%s Expired)", kind),
			Details:  fmt.Sprintf("name=%s", name),
		})

	default:
		c.hub.RecordEvent(FlowEvent{
			Type:    EventNodeRemoved,
			From:    c.authNodeID,
			To:      c.authNodeID,
			Summary: fmt.Sprintf("OpDelete(%s)", kind),
			Details: fmt.Sprintf("kind=%s name=%s", kind, name),
		})
	}
}

func classifyResourceEventType(kind string) EventType {
	switch kind {
	case types.KindAccessList, types.KindAccessListMember, types.KindAccessListReview,
		types.KindAccessMonitoringRule, types.KindCrownJewel, types.KindAccessGraphSettings:
		return EventAccessRequest
	case types.KindCertAuthorityOverride, types.KindRotatedKey, types.KindPendingCSRRequest,
		types.KindSPIFFEFederation, types.KindSAMLIdPServiceProvider, types.KindWorkloadIdentity,
		types.KindRecordingEncryption:
		return EventCARotation
	case types.KindUserLoginState, scopedaccess.KindScopedRole, scopedaccess.KindScopedRoleAssignment,
		types.KindUserGroup, types.KindStaticTokens, types.KindStaticScopedTokens,
		types.KindStaticHostUser, types.KindDevice, types.KindOktaImportRule, types.KindOktaAssignment:
		return EventRBACChange
	case types.KindHeadlessAuthentication, types.KindValidatedMFAChallenge,
		types.KindKubeWaitingContainer, types.KindBeam:
		return EventLiveSession
	case types.KindRelayServer, types.KindGitServer, types.KindBotInstance,
		types.KindLinuxDesktop, types.KindDynamicWindowsDesktop, types.KindDatabaseService:
		return EventHeartbeat
	default:
		return EventConfigChange
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
