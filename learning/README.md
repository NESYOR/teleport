# Teleport Architecture & Workflows Learning Guide

This folder contains structured guides and sequence diagrams explaining the architecture and core workflows of **Teleport's Auth and Proxy services**, with direct links to the codebase.

## Contents

1. [**01 - Process Startup & Service Lifecycle**](./01-process-startup-and-lifecycle.md)
   * How a single `teleport` binary orchestrates Auth, Proxy, and Agent services using `TeleportProcess` and an internal event bus.
   * How caching (`accessPoint`) keeps the Proxy fast and resilient.
2. [**02 - Auth Service Request Pipeline**](./02-auth-service-pipeline.md)
   * How every request to the Auth Service is authenticated via mTLS, authorized via RBAC, and executed against the Certificate Authorities and storage backend.
3. [**03 - Proxy Service & ALPN/TLS Routing**](./03-proxy-service-and-alpn-routing.md)
   * How the stateless Proxy multiplexes SSH, HTTPS, gRPC, Kubernetes, and Database traffic over a single TLS port using ALPN and SNI.
   * How Reverse Tunnels and Proxy Peering route traffic to private agents.
4. [**04 - User Authentication Flow (`tsh login` & Web UI)**](./04-user-authentication-flow.md)
   * Step-by-step walkthrough of local (Password + WebAuthn/MFA) and SSO (OIDC/SAML/GitHub) login flows and short-lived SSH/TLS certificate issuance.
5. [**05 - Agent Joining, Heartbeats & Reverse Tunnels**](./05-agent-joining-and-heartbeats.md)
   * How Nodes, Database Agents, Kubernetes Agents, and Proxies join a cluster, rotate host certificates, maintain the Inventory Control Stream, and dial reverse tunnels.
6. [**06 - Resource Connection Flow (`tsh ssh` / DB / Kube)**](./06-resource-connection-flow.md)
   * Step-by-step walkthrough of how an authenticated client connects through the Proxy to a target resource, including tunnel resolution, end-to-end RBAC, and session recording.
7. [**07 - Live Cluster Topology & Packet Flow Map**](./07-live-topology-map.md)
   * Interactive HTML5 Canvas + SSE visualizer (`lib/observability/livemap`) and telemetry tap points showing live heartbeats, cache watcher fan-out, and gRPC/RBAC flows.

---

## Core Package Map

| Package | Responsibility | Key Entrypoint |
| :--- | :--- | :--- |
| [`lib/service`](file:///home/strawhat/project/teleport/lib/service) | Process supervisor, service wiring, lifecycle, and local caching | [`TeleportProcess`](file:///home/strawhat/project/teleport/lib/service/service.go#L618-L756) |
| [`lib/auth`](file:///home/strawhat/project/teleport/lib/auth) | Certificate Authority, authentication, gRPC API, RBAC wrapper | [`auth.Server`](file:///home/strawhat/project/teleport/lib/auth/auth.go#L1311-L1435), [`ServerWithRoles`](file:///home/strawhat/project/teleport/lib/auth/auth_with_roles.go#L95-L140) |
| [`lib/authz`](file:///home/strawhat/project/teleport/lib/authz) | Role-Based Access Control (RBAC) evaluation engine | [`Authorizer`](file:///home/strawhat/project/teleport/lib/authz/permissions.go#L167-L170) |
| [`lib/srv/alpnproxy`](file:///home/strawhat/project/teleport/lib/srv/alpnproxy) | Single-port TLS routing via ALPN & SNI inspection | [`alpnproxy.Proxy`](file:///home/strawhat/project/teleport/lib/srv/alpnproxy/proxy.go#L246-L270) |
| [`lib/proxy`](file:///home/strawhat/project/teleport/lib/proxy) | Connection routing across nodes, reverse tunnels, and peer proxies | [`proxy.Router`](file:///home/strawhat/project/teleport/lib/proxy/router.go#L179-L205) |
| [`lib/reversetunnel`](file:///home/strawhat/project/teleport/lib/reversetunnel) | Inbound SSH tunnel server for dial-back agents and leaf clusters | [`reversetunnel.server`](file:///home/strawhat/project/teleport/lib/reversetunnel/srv.go#L87-L135) |
| [`lib/web`](file:///home/strawhat/project/teleport/lib/web) | Web UI HTTP REST API, `/webapi/ping`, SSO redirects, WebSockets | [`web.APIServer`](file:///home/strawhat/project/teleport/lib/web/apiserver.go#L194-L235) |
| [`lib/cache`](file:///home/strawhat/project/teleport/lib/cache) | In-memory watch-based cache (`accessPoint`) used by Proxy and Agents | [`cache.Cache`](file:///home/strawhat/project/teleport/lib/cache/cache.go) |
| [`lib/backend`](file:///home/strawhat/project/teleport/lib/backend) | Persistent key-value storage abstraction for Auth | [`backend.Backend`](file:///home/strawhat/project/teleport/lib/backend/backend.go) |
