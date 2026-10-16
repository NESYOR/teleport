/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 */

package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	apievents "github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/lib/auth/authtest"
	"github.com/gravitational/teleport/lib/observability/livemap"
)

func TestLiveMapVisualizer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start a real in-memory Auth Server with mTLS gRPC listener
	srv := newTestTLSServer(t)

	// 2. Start the Auth Event Consumer watching real cluster events over gRPC (NewWatcher)
	hub := livemap.NewHub()
	consumer := livemap.NewConsumer(hub)

	watcherClient, err := srv.NewClient(authtest.TestAdmin())
	require.NoError(t, err)
	defer watcherClient.Close()

	go func() {
		_ = consumer.WatchCluster(ctx, watcherClient)
	}()

	// Give the NewWatcher stream a moment to initialize (OpInit)
	time.Sleep(150 * time.Millisecond)

	// 3. Connect an SSH Node over a real InventoryControlStream
	const (
		authID  = "auth-primary-01"
		proxyID = "proxy-edge-01"
		nodeID  = "ssh-node-01"
		dbName  = "prod-postgres"
	)

	nodeClient, err := srv.NewClient(authtest.TestServerID(types.RoleNode, nodeID))
	require.NoError(t, err)
	defer nodeClient.Close()

	stream, err := nodeClient.InventoryControlStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	err = stream.Send(ctx, proto.UpstreamInventoryHello_builder{
		ServerID: nodeID,
		Version:  teleport.Version,
		Services: []string{string(types.RoleNode)},
	}.Build())
	require.NoError(t, err)

	// Seed initial Auth Server and Proxy Server presence into Auth backend
	// so the Consumer discovers them via OpPut(KindAuthServer) and OpPut(KindProxy).
	authServerRes, err := types.NewServer(authID, types.KindAuthServer, types.ServerSpecV2{
		Addr:     "10.0.0.10:3025",
		Hostname: "auth-primary-01.internal",
		Version:  teleport.Version,
	})
	require.NoError(t, err)
	require.NoError(t, srv.Auth().UpsertAuthServer(ctx, authServerRes))

	proxyServerRes, err := types.NewServer(proxyID, types.KindProxy, types.ServerSpecV2{
		Addr:        "10.0.0.20:3023",
		PublicAddrs: []string{"teleport.example.com:443"},
		PeerAddr:    "10.0.0.20:3021",
		Hostname:    "proxy-edge-01.dmz",
		Version:     teleport.Version,
	})
	require.NoError(t, err)
	_, err = srv.Auth().UpsertProxyServer(ctx, proxyServerRes)
	require.NoError(t, err)

	dbServerRes, err := types.NewDatabaseServerV3(types.Metadata{
		Name: "db-agent-01",
	}, types.DatabaseServerSpecV3{
		HostID:   "db-host-01",
		Hostname: "db-agent-01.internal",
		Database: &types.DatabaseV3{
			Metadata: types.Metadata{Name: dbName},
			Spec: types.DatabaseSpecV3{
				Protocol: "postgres",
				URI:      "postgres.internal:5432",
			},
		},
	})
	require.NoError(t, err)
	_, err = watcherClient.UpsertDatabaseServer(ctx, dbServerRes)
	require.NoError(t, err)

	// Define real cluster event actions that mutate Auth state so the Consumer
	// receives them via NewWatcher and dynamically updates the topology graph.
	triggerAction := func(action string) (string, error) {
		switch action {
		case "heartbeat":
			nodeSrv, err := types.NewServer(nodeID, types.KindNode, types.ServerSpecV2{
				Addr:      "10.0.1.25:3022",
				Hostname:  "prod-ssh-01",
				UseTunnel: true,
			})
			if err != nil {
				return "", err
			}
			if err := stream.Send(ctx, proto.InventoryHeartbeat_builder{
				SSHServer: nodeSrv.(*types.ServerV2),
			}.Build()); err != nil {
				return "", err
			}
			return "Node sent InventoryHeartbeat -> Auth emitted OpPut(KindNode)", nil

		case "tunnel":
			tc, err := types.NewTunnelConnection(nodeID, types.TunnelConnectionSpecV2{
				ClusterName:   "root-cluster",
				ProxyName:     proxyID,
				LastHeartbeat: time.Now().UTC(),
				Type:          types.NodeTunnel,
			})
			if err != nil {
				return "", err
			}
			if err := srv.Auth().UpsertTunnelConnection(ctx, tc); err != nil {
				return "", err
			}
			return "Upserted TunnelConnection -> Auth emitted OpPut(KindTunnelConnection) linking node:ssh-node-01 -> proxy:proxy-edge-01", nil

		case "session":
			st, err := types.NewSessionTracker(types.SessionTrackerSpecV1{
				SessionID:   uuid.NewString(),
				Kind:        string(types.SSHSessionKind),
				State:       types.SessionState_SessionStateRunning,
				Hostname:    "prod-ssh-01",
				HostID:      nodeID,
				ClusterName: "root-cluster",
				Login:       "ubuntu",
				Participants: []types.Participant{
					{ID: uuid.NewString(), User: "alice", Mode: string(types.SessionPeerMode)},
				},
			})
			if err != nil {
				return "", err
			}
			if _, err := nodeClient.CreateSessionTracker(ctx, st); err != nil {
				return "", err
			}
			if err := consumer.SyncActiveSessions(ctx, nodeClient); err != nil {
				return "", err
			}
			return "Created SessionTracker -> SyncActiveSessions linked user:alice -> node:ssh-node-01", nil

		case "access_request":
			user, err := types.NewUser("alice")
			if err == nil {
				user.SetRoles([]string{"access"})
				_, _ = srv.Auth().UpsertUser(ctx, user)
			}
			req, err := types.NewAccessRequest(uuid.NewString(), "alice", "access")
			if err != nil {
				return "", err
			}
			req.SetRequestReason("Investigating production incident #402")
			if err := srv.Auth().UpsertAccessRequest(ctx, req); err != nil {
				return "", err
			}
			return "Upserted AccessRequest -> Auth emitted OpPut(KindAccessRequest) for user:alice -> auth", nil

		case "lock":
			lock, err := types.NewLock("incident-lock-"+uuid.NewString()[:6], types.LockSpecV2{
				Target: types.LockTarget{
					ServerID: nodeID,
				},
				Message: "Compromised host quarantined by SecOps",
			})
			if err != nil {
				return "", err
			}
			if err := srv.Auth().UpsertLock(ctx, lock); err != nil {
				return "", err
			}
			return "Upserted Lock -> Auth emitted OpPut(KindLock) locking node:ssh-node-01", nil

		case "audit":
			consumer.ProcessAuditEvent(&apievents.SessionStart{
				Metadata: apievents.Metadata{
					Type:        "session.start",
					Code:        "T2000I",
					Time:        time.Now().UTC(),
					ClusterName: "root-cluster",
				},
				UserMetadata: apievents.UserMetadata{
					User: "alice",
				},
				ServerMetadata: apievents.ServerMetadata{
					ServerID:        nodeID,
					ServerNamespace: "default",
				},
			})
			return "Consumed AuditEvent(session.start) for user:alice -> node:ssh-node-01", nil

		case "expire_node":
			if err := srv.Auth().DeleteAllNodes(ctx, "default"); err != nil {
				return "", err
			}
			return "Deleted ssh-node-01 -> Auth emitted OpDelete(KindNode) removing node from live map", nil

		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	}

	// Trigger real Auth state mutations and let Consumer build the map from NewWatcher
	_, err = triggerAction("heartbeat")
	require.NoError(t, err)

	_, err = triggerAction("tunnel")
	require.NoError(t, err)

	_, err = triggerAction("session")
	require.NoError(t, err)

	_, err = triggerAction("access_request")
	require.NoError(t, err)

	_, err = triggerAction("lock")
	require.NoError(t, err)

	_, err = triggerAction("audit")
	require.NoError(t, err)

	// Wait for NewWatcher stream to deliver all OpPut events to Consumer
	require.Eventually(t, func() bool {
		snap := hub.GetSnapshot()
		return len(snap.Nodes) >= 5 && len(snap.Edges) >= 8
	}, 5*time.Second, 50*time.Millisecond)

	handler := livemap.NewHTTPHandler(hub, triggerAction)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	snap := hub.GetSnapshot()
	t.Logf("\n================ DYNAMIC TOPOLOGY BUILT FROM AUTH EVENTS ================")
	t.Logf("Discovered Nodes (%d):", len(snap.Nodes))
	for _, n := range snap.Nodes {
		t.Logf("  * [%s] id=%s label=%q addr=%q", n.Kind, n.ID, n.Label, n.Addr)
	}
	t.Logf("Discovered Directed Edges (%d):", len(snap.Edges))
	for _, e := range snap.Edges {
		t.Logf("  * %s ──► %s | Events: %d | Types: %v | Last: %s",
			e.From, e.To, e.TotalCount, e.ByType, e.LastEvent)
	}
	t.Logf("=========================================================================")

	if os.Getenv("LIVEMAP_SERVE") == "1" {
		addr := "127.0.0.1:8899"
		t.Logf("Serving Live Auth Event Consumer Map at http://%s (Press Ctrl+C to stop)", addr)
		_ = http.ListenAndServe(addr, handler)
	}
}
