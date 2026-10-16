/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 */

// Package livemap provides a real-time cluster topology and control-plane packet
// flow tap and interactive SSE/HTML visualizer built dynamically from Teleport
// Auth resource watch events and audit events.
package livemap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// EventType identifies the kind of Auth resource event, heartbeat, tunnel, session, or RPC.
type EventType string

const (
	EventGRPCUnary     EventType = "GRPC_UNARY"
	EventGRPCStream    EventType = "GRPC_STREAM"
	EventAuthorizer    EventType = "AUTHORIZER"
	EventRBACCheck     EventType = "RBAC_CHECK"
	EventControlStream EventType = "CONTROL_STREAM"
	EventHeartbeat     EventType = "HEARTBEAT"
	EventInstanceHB    EventType = "INSTANCE_HB"
	EventWatchEvent    EventType = "WATCH_EVENT"
	EventTunnelHB      EventType = "TUNNEL_HB"
	EventLiveSession   EventType = "LIVE_SESSION"
	EventAccessRequest EventType = "ACCESS_REQUEST"
	EventLock          EventType = "SESSION_LOCK"
	EventCARotation    EventType = "CA_ROTATION"
	EventRBACChange    EventType = "RBAC_CHANGE"
	EventConfigChange  EventType = "CONFIG_CHANGE"
	EventAudit         EventType = "AUDIT_EVENT"
	EventNodeRemoved   EventType = "RESOURCE_EXPIRED"
)

// Node represents a dynamically discovered component or user in the cluster topology.
type Node struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind"` // "auth", "proxy", "node", "db", "kube", "app", "user"
	Addr     string            `json:"addr,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Expiry   time.Time         `json:"expiry,omitempty"`
	LastSeen time.Time         `json:"last_seen"`
}

// EdgeStat aggregates traffic or relationship state between two nodes in the topology.
type EdgeStat struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	TotalCount uint64            `json:"total_count"`
	ByType     map[string]uint64 `json:"by_type"`
	LastEvent  string            `json:"last_event"`
	LastSeen   time.Time         `json:"last_seen"`
}

// FlowEvent represents a single observed Auth watch event, tunnel heartbeat, session event, or RPC.
type FlowEvent struct {
	Seq       uint64    `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Type      EventType `json:"type"`
	From      string    `json:"from"`
	FromLabel string    `json:"from_label,omitempty"`
	FromKind  string    `json:"from_kind,omitempty"`
	FromAddr  string    `json:"from_addr,omitempty"`
	To        string    `json:"to"`
	ToLabel   string    `json:"to_label,omitempty"`
	ToKind    string    `json:"to_kind,omitempty"`
	Summary   string    `json:"summary"`
	Details   string    `json:"details,omitempty"`
	Allowed   *bool     `json:"allowed,omitempty"`
}

// Snapshot represents the current state of the topology graph and recent events.
type Snapshot struct {
	Nodes        []Node      `json:"nodes"`
	Edges        []EdgeStat  `json:"edges"`
	RecentEvents []FlowEvent `json:"recent_events"`
}

// Hub collects topology nodes, edge counters, and live flow events dynamically
// from Auth events and broadcasts them to connected SSE clients.
type Hub struct {
	enabled atomic.Bool
	seq     atomic.Uint64

	mu           sync.RWMutex
	nodes        map[string]Node
	edges        map[string]*EdgeStat
	recentEvents []FlowEvent
	maxRecent    int
	subscribers  map[chan FlowEvent]struct{}
}

// NewHub creates a new empty topology Hub (no static/hardcoded nodes).
func NewHub() *Hub {
	return &Hub{
		nodes:       make(map[string]Node),
		edges:       make(map[string]*EdgeStat),
		maxRecent:   250,
		subscribers: make(map[chan FlowEvent]struct{}),
	}
}

// DefaultHub is the process-wide topology tap hub.
var DefaultHub = NewHub()

// Enable turns on event collection on the DefaultHub.
func Enable() {
	DefaultHub.enabled.Store(true)
}

// Disable turns off event collection on the DefaultHub.
func Disable() {
	DefaultHub.enabled.Store(false)
}

// Enabled reports whether DefaultHub is actively recording events.
func Enabled() bool {
	return DefaultHub.enabled.Load()
}

// Reset clears all nodes, edges, and events in the Hub.
func (h *Hub) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	clear(h.nodes)
	clear(h.edges)
	h.recentEvents = nil
}

// UpsertNode adds or updates a node discovered from an Auth resource event.
func (h *Hub) UpsertNode(n Node) {
	if n.ID == "" {
		return
	}
	if n.LastSeen.IsZero() {
		n.LastSeen = time.Now()
	}
	if n.Label == "" {
		n.Label = n.ID
	}
	h.mu.Lock()
	existing, ok := h.nodes[n.ID]
	if ok {
		if n.Kind == "" {
			n.Kind = existing.Kind
		}
		if n.Addr == "" {
			n.Addr = existing.Addr
		}
		if n.Label == n.ID && existing.Label != "" {
			n.Label = existing.Label
		}
	}
	h.nodes[n.ID] = n
	h.mu.Unlock()
}

// RemoveNode removes a node and its incident edges when an Auth OpDelete event arrives.
func (h *Hub) RemoveNode(id string) {
	h.mu.Lock()
	delete(h.nodes, id)
	for k, e := range h.edges {
		if e.From == id || e.To == id {
			delete(h.edges, k)
		}
	}
	h.mu.Unlock()
}

// RemoveEdge removes a specific directed edge (e.g. when a TunnelConnection or SessionTracker expires).
func (h *Hub) RemoveEdge(from, to string) {
	h.mu.Lock()
	delete(h.edges, from+"->"+to)
	h.mu.Unlock()
}

// Record records a FlowEvent on the DefaultHub if enabled.
func Record(ev FlowEvent) {
	if !DefaultHub.enabled.Load() {
		return
	}
	DefaultHub.RecordEvent(ev)
}

// RecordEvent stores a FlowEvent, updates the topology nodes and edges, and notifies subscribers.
func (h *Hub) RecordEvent(ev FlowEvent) {
	now := time.Now()
	ev.Seq = h.seq.Add(1)
	if ev.Timestamp.IsZero() {
		ev.Timestamp = now
	}
	if ev.From == "" {
		ev.From = "unknown"
	}
	if ev.To == "" {
		ev.To = "auth-server"
	}

	h.mu.Lock()
	// Upsert From node
	fromNode, ok := h.nodes[ev.From]
	if !ok {
		kind := ev.FromKind
		if kind == "" {
			kind = inferKind(ev.From)
		}
		label := ev.FromLabel
		if label == "" {
			label = ev.From
		}
		fromNode = Node{ID: ev.From, Label: label, Kind: kind, Addr: ev.FromAddr}
	}
	if ev.FromKind != "" {
		fromNode.Kind = ev.FromKind
	}
	if ev.FromLabel != "" {
		fromNode.Label = ev.FromLabel
	}
	if ev.FromAddr != "" {
		fromNode.Addr = ev.FromAddr
	}
	fromNode.LastSeen = now
	h.nodes[ev.From] = fromNode

	// Upsert To node
	toNode, ok := h.nodes[ev.To]
	if !ok {
		kind := ev.ToKind
		if kind == "" {
			kind = inferKind(ev.To)
		}
		label := ev.ToLabel
		if label == "" {
			label = ev.To
		}
		toNode = Node{ID: ev.To, Label: label, Kind: kind}
	}
	if ev.ToKind != "" {
		toNode.Kind = ev.ToKind
	}
	if ev.ToLabel != "" {
		toNode.Label = ev.ToLabel
	}
	toNode.LastSeen = now
	h.nodes[ev.To] = toNode

	// Update directed edge stats
	edgeKey := ev.From + "->" + ev.To
	edge, ok := h.edges[edgeKey]
	if !ok {
		edge = &EdgeStat{
			From:   ev.From,
			To:     ev.To,
			ByType: make(map[string]uint64),
		}
		h.edges[edgeKey] = edge
	}
	edge.TotalCount++
	edge.ByType[string(ev.Type)]++
	edge.LastEvent = ev.Summary
	edge.LastSeen = now

	// Append to ring buffer
	h.recentEvents = append(h.recentEvents, ev)
	if len(h.recentEvents) > h.maxRecent {
		h.recentEvents = h.recentEvents[len(h.recentEvents)-h.maxRecent:]
	}

	// Fan-out to SSE subscribers without blocking
	for ch := range h.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
	h.mu.Unlock()
}

// GetSnapshot returns a point-in-time copy of all nodes, edges, and recent events.
func (h *Hub) GetSnapshot() Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	snap := Snapshot{
		Nodes:        make([]Node, 0, len(h.nodes)),
		Edges:        make([]EdgeStat, 0, len(h.edges)),
		RecentEvents: make([]FlowEvent, len(h.recentEvents)),
	}
	for _, n := range h.nodes {
		snap.Nodes = append(snap.Nodes, n)
	}
	for _, e := range h.edges {
		byTypeCopy := make(map[string]uint64, len(e.ByType))
		for k, v := range e.ByType {
			byTypeCopy[k] = v
		}
		snap.Edges = append(snap.Edges, EdgeStat{
			From:       e.From,
			To:         e.To,
			TotalCount: e.TotalCount,
			ByType:     byTypeCopy,
			LastEvent:  e.LastEvent,
			LastSeen:   e.LastSeen,
		})
	}
	copy(snap.RecentEvents, h.recentEvents)
	return snap
}

func inferKind(name string) string {
	switch name {
	case "auth-server", "Auth":
		return "auth"
	case "Proxy", "proxy":
		return "proxy"
	case "Node", "node":
		return "node"
	case "Db", "Database":
		return "db"
	case "Kube":
		return "kube"
	default:
		return "user"
	}
}

// NewHTTPHandler returns an http.Handler serving the interactive topology map UI,
// JSON snapshot endpoint, SSE live stream, and optional action trigger endpoint.
func NewHTTPHandler(h *Hub, trigger func(action string) (string, error)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.GetSnapshot())
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := make(chan FlowEvent, 64)
		h.mu.Lock()
		h.subscribers[ch] = struct{}{}
		h.mu.Unlock()
		defer func() {
			h.mu.Lock()
			delete(h.subscribers, ch)
			h.mu.Unlock()
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev := <-ch:
				data, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/trigger", func(w http.ResponseWriter, r *http.Request) {
		if trigger == nil {
			http.Error(w, "no trigger configured", http.StatusNotImplemented)
			return
		}
		action := r.URL.Query().Get("action")
		msg, err := trigger(action)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardHTML))
	})

	return mux
}
