# Events Microservice Migration Plan

This document lays out a step-by-step plan to migrate the Events domain out of the monolith into a dedicated gRPC-backed microservice while keeping existing REST/WebSocket behavior intact. Each phase is designed to be executed independently so future sessions can pick up from any stage without re-reading the entire codebase.

---

## Overview

- **Disclaimer**: **All future service-to-service calls must use gRPC; REST/WebSocket interfaces remain only for external clients.**

- **Goal**: Extract Events-related repositories/services/controllers into a standalone service (`cmd/events`) that serves both REST (for clients) and gRPC (for internal callers), while the API gateway and WebSocket hub continue to expose `/api/events` and event RSVP updates.
- **Scope**: CRUD operations for events, invitations, discussion posts, RSVP handling, event recommendations, and associated cache logic.
- **Constraints**:
  - No breaking changes to public HTTP endpoints or WebSocket payloads.
  - Reuse existing Mongo collections, Redis caches, and Kafka topics initially.
  - Add gRPC contracts for internal consumers (gateway, notifications, etc.).
  - Maintain the existing websocket-based broadcasting of RSVP updates via the hub; only the source of those events moves.

---

## Phase 1 – Preparation

- [x] **Inventory current components**
  - *Repositories*: `EventRepository` (Mongo collection `events`), `EventInvitationRepository` (`event_invitations`), `EventPostRepository`.
  - *Services*: `EventService` (business logic, RSVP), `EventRecommendationService` (friends/trending logic), `EventCache` (Redis), realtime RSVP gRPC bridge.
  - *Controllers*: `EventController` (all `/api/events` HTTP handlers).
  - *Routes*: `/api/events` group in `internal/server/router.go` (CRUD, RSVPs, invitations, posts, trending, recommendations, nearby).
  - *WebSocket*: `Hub.EventRSVPEvents` channel; a realtime gRPC server accepts RSVP events and the hub broadcasts to clients.

- [x] **Document dependencies**
  - *MongoDB*: collections `events`, `event_invitations`, `event_posts`, geospatial indexes for nearby queries.
  - *Redis*: `EventCache` (stats, RSVP status, trending) plus Redis-backed rate limit middleware (RSVP/create/invite/search).
  - *Graph DB*: Neo4j via `EventGraphRepository` for friends-going queries.
  - *Kafka*: notifications emitted via `NotificationService`; potential event updates to feed subsystem.
  - *External services*: Notification service (Kafka), WebSocket hub (direct channel), Auth/user data for permissions, Search uses `EventService`.

- [x] **Define service boundary**
  - Events microservice owns all event CRUD, RSVP, invitations, posts, categories, recommendations, trending, nearby searches, and associated caching/seeding.
  - API gateway remains the external REST/WebSocket interface; it authenticates/authorizes, applies rate limits, and forwards to Events service via gRPC.
  - WebSocket hub stays centralized; Events service reports RSVPs through a dedicated gRPC call (or Kafka fallback) so hub can notify clients.

- [x] **Design gRPC API**
  - Draft `proto/events.proto` including RPCs mirroring every REST endpoint:
    - `CreateEvent`, `GetEvent`, `UpdateEvent`, `DeleteEvent`, `ListEvents`, `GetMyEvents`, `GetBirthdays`, `GetCategories`, `GetNearbyEvents`, `SearchEvents`, `ShareEvent`.
    - Invitation flows: `InviteFriends`, `GetInvitations`, `RespondToInvitation`.
    - Posts: `CreatePost`, `GetPosts`, `DeletePost`, `ReactToPost`.
    - Attendance: `RSVP`, `GetAttendees`, `AddCoHost`, `RemoveCoHost`.
    - Recommendations: `GetRecommendations`, `GetTrending`.
    - RSVP broadcasting RPC: `ReportRSVPEvent` (Events service → Gateway/WebSocket).
  - Use unary RPCs for REST-style calls; consider server-streaming only if future UIs need live updates.
  - Place compiled stubs under `pkg/proto/events`; enforce metadata for user identity (JWT claims) in gRPC context.

---

## Phase 2 – Build Events Service Skeleton

- [x] **New binary**
  - `cmd/events/main.go` now boots a dedicated gRPC server (`events.NewGRPCServer`) on `cfg.EventsGRPCPort`, registers reflection, and handles SIGINT/SIGTERM for graceful shutdown.
  - `internal/events/bootstrap.go` wires Mongo, Redis, optional Neo4j, repositories, `EventService`, `EventRecommendationService`, and Redis-backed caches using the shared `server.Init*` helpers.
  - A `NoopBroadcaster` lives under `internal/events` as the temporary RSVP bridge until the gateway-side gRPC client is introduced.

- [x] **Relocate code**
  - Protobuf contract (`proto/events/v1/events.proto`) now mirrors the HTTP payloads (user short views, invitations, categories, recommendations, pagination metadata, etc.) and regenerates into `pkg/proto/events/v1`.
  - `internal/events/server.go` implements every RPC (CRUD, invitations, posts, attendees, search, recommendations, nearby) by delegating to the existing `EventService` and `EventRecommendationService`, preserving response shapes through the conversion helpers in `internal/events/converters.go`.
  - REST controllers still run in-process on the gateway, but they can now be proxied to the Events service once the gRPC client is wired in Phase 3.

- [x] **WebSocket integration**
  - Added `proto/realtime/v1/realtime.proto` plus a gateway-side gRPC server (`internal/realtime`) that receives `ReportRSVPEvent` requests and pushes them onto `hub.EventRSVPEvents`.
  - Events service now uses `RealtimeBroadcaster` (dialed via `REALTIME_GRPC_HOST`/`PORT`) so RSVPs are immediately forwarded through the gateway before WebSocket fan-out; it falls back to `NoopBroadcaster` if the realtime endpoint is unavailable.

- [x] **Rate limiting & auth**
  - Gateway continues to enforce the Redis-backed event-specific throttling middleware (`EventRateLimiter` for RSVP/invite/search/etc.) before invoking gRPC calls, so the same limits apply without duplication.
  - Authentication remains centralized in the HTTP layer; controllers now simply forward authenticated user IDs to the Events service.

- [x] **Observability**
  - Events service exposes `/metrics` on `EVENTS_METRICS_PORT` and wraps all RPCs with Prometheus/structured logging interceptors to record latency, totals, and error codes.
  - These metrics can be scraped alongside the gateway for full-service dashboards.

---

## Phase 3 – Gateway Integration

- [x] **gRPC client setup**
  - Gateway now dials the Events microservice via `internal/eventsclient.Client` (host/port supplied via `EVENTS_GRPC_HOST/PORT`).
  - `controllers.EventController` depends on the new `services.EventServiceContract`/`EventRecommendationServiceContract` interfaces, letting the HTTP layer proxy every endpoint through gRPC without rewriting handlers.
  - `pkg/utils.GetStatusCode` understands gRPC status codes so HTTP responses stay aligned with previous error semantics.

- [x] **WebSocket RSVP flow**
  - Gateway now exposes a dedicated gRPC listener (`REALTIME_GRPC_PORT`) that receives RSVP events and forwards them to the hub, preserving payload structure.
  - Events service replaces the in-process broadcaster with a gRPC client, keeping the existing hub/broadcast logic untouched.

- [x] **Feature flag / dual path**
  - Gateway always proxies to the Events microservice via gRPC; the legacy in-process code path has been removed to avoid configuration drift.
  - Configuration now focuses on service discovery (host/port) rather than toggling execution modes.

- [x] **Update router**
  - All `/api/events` routes still use the Gin controller layer, but those controllers now delegate to the gRPC client (`services.EventServiceContract` implemented by `eventsclient.Client`), so every HTTP call is proxied over gRPC before touching the domain.
  - Rate-limiting middleware remains in place on the gateway and executes before the gRPC call, maintaining the same protections without duplicating logic inside the service.

---

## Phase 4 – Data & Background Tasks

- [x] **Cache warmers & seeding**
  - `events.StartCacheWarmers` now refreshes trending event IDs and category aggregates every few minutes using the Redis-backed `EventCache`, so initial requests hit warm caches even before clients make requests.
  - `EventService.GetCategories` reads/writes the shared cache, ensuring the gateway and microservice continue using identical Redis keys; marketplace seeding remains out of scope.

- [x] **Recommendations**
  - The existing `EventRecommendationService` is instantiated inside the Events microservice and exposed via the gRPC endpoints (`GetRecommendations`, `GetTrending`), so every consumer—gateway or other services—pulls from the same logic/cache.
  - Trending recommendations are also fed into the cache warmers so traffic spikes don’t cause repeated recomputation.

- [x] **Notifications integration**
  - Event actions continue to publish notifications through the shared `NotificationRepository`/Kafka producer that the microservice reuses during bootstrap, so RSVP/invite flows still emit the same events consumed by the notification pipeline.
  - Future extraction of the Notification service can subscribe to the same Kafka topics without changes to the Events API contract.

---

## Phase 5 – Deployment & Rollout

- [x] **Containerization**
  - Added `Dockerfile.events` plus a dedicated `events-service` entry in `docker-compose.yml`, including port mappings (`9096/9100`) and realtime host wiring so the gateway and Events service can run side-by-side locally.
  - New Kubernetes manifest (`k8s/events-deployment.yaml`) defines the Deployment, Service, and HorizontalPodAutoscaler for the Events gRPC deployment; the Service exposes both gRPC and metrics ports for service discovery/scraping.

- [x] **CI/CD**
  - GitHub Actions pipeline now builds/tests `cmd/events` (unit tests + linux build) and publishes both the gateway and events Docker images.
  - Additional unit tests (e.g., realtime server) are included in the pipeline; future integration tests can be layered in without pipeline changes.

- [ ] **Staged rollout**
  - Deploy Events service alongside monolith.
  - Enable feature flag on staging, run regression tests through REST + WebSocket flows.
  - Switch production traffic gradually; monitor metrics (latency, errors, RSVP flows).

- [ ] **Cleanup**
  - After stability confirmed, remove Events code from gateway (repositories/services/controllers).
  - Document new architecture and update diagrams (README, docs).

---

## WebSocket RSVP Handling Strategy

- **Current behavior**: Events service completes RSVP logic, then calls the gateway’s realtime gRPC endpoint which enqueues onto `hub.EventRSVPEvents`; `hub_runtime.go` broadcasts via WebSockets.
- **New design**:
  1. Events service handles RSVP via gRPC request; once persisted, it calls `BroadcastRSVP` RPC on the gateway (or publishes to Kafka).
  2. Gateway receives the RPC, wraps it into `models.EventRSVPEvent`, and pushes onto `hub.EventRSVPEvents`.
  3. Existing hub logic sends updates to connected clients; no change to payload or delivery semantics.
- **Fallback**: If gRPC link fails, Events service can optionally enqueue RSVP updates to Kafka `event_rsvp_updates` topic; gateway consumes and forwards to hub (adds resilience).

---

## Work Breakdown Summary

1. **Contracts & skeleton**
   - [x] Define protobufs, generate code, set up gRPC server/client scaffolding.
2. **Service extraction**
   - [x] Move repos/services/cache/seed logic into new binary; implement gRPC handlers.
3. **Gateway proxy**
   - [x] Replace `/api/events` handlers with gRPC proxy logic, including metadata, rate limiting, and error translation.
4. **WebSocket bridge**
   - [x] Add gRPC endpoint for RSVP broadcasts or Kafka fallback.
5. **Deployment artifacts**
   - [ ] Dockerfile, Kubernetes deployment/service/hpa, CI pipeline updates (Docker/K8s done; CI still pending).
6. **Rollout**
   - [ ] Feature flag, staging validation, production cutover, legacy cleanup.

This plan ensures every future session knows exactly which phase to tackle next and how the WebSocket RSVP path will be handled throughout the migration.***
