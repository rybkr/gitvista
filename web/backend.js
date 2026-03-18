import { wsUrl } from "./apiBase.js";
import { telemetryStore } from "./telemetry.js";

const RECONNECT_DELAY_INITIAL_MS = 1000;
const RECONNECT_DELAY_MAX_MS = 30000;

export function startBackend({
    onDelta,
    onBootstrapChunk,
    onBootstrapComplete,
    onStatus,
    onHead,
    onRepoMetadata,
    onConnectionStateChange,
    logger,
}) {
    return openWebSocket({
        onBootstrapChunk,
        onBootstrapComplete,
        onDelta,
        onStatus,
        onHead,
        onRepoMetadata,
        onConnectionStateChange,
        logger,
    });
}

function openWebSocket({
    onDelta,
    onBootstrapChunk,
    onBootstrapComplete,
    onStatus,
    onHead,
    onRepoMetadata,
    onConnectionStateChange,
    logger,
}) {
    const url = wsUrl();
    logger?.info("Opening WebSocket connection", url);

    let reconnectDelay = RECONNECT_DELAY_INITIAL_MS;
    let reconnectAttempt = 0;
    let destroyed = false;
    let socket = null;
    let reconnectTimer = null;

    function notifyState(state, attempt) {
        telemetryStore.recordConnectionState(state, attempt || 0);
        onConnectionStateChange?.(state, attempt);
    }

    function handleMessage(payload) {
        switch (payload?.type) {
            case "repoSummary":
                telemetryStore.recordRepoSummary(payload.repo);
                onRepoMetadata?.(payload.repo);
                break;
            case "graphBootstrapChunk":
                telemetryStore.recordBootstrapChunk(payload.bootstrap);
                onBootstrapChunk?.(payload.bootstrap);
                break;
            case "bootstrapComplete":
                telemetryStore.recordBootstrapComplete(payload.bootstrapComplete);
                onBootstrapComplete?.(payload.bootstrapComplete);
                break;
            case "graphDelta":
                telemetryStore.recordDelta(payload.delta);
                onDelta?.(payload.delta);
                break;
            case "status":
                onStatus?.(payload.status);
                break;
            case "head":
                onHead?.(payload.head);
                break;
            default:
                logger?.warn("Unknown WebSocket payload type", payload?.type);
        }
    }

    function connect() {
        if (destroyed) return;
        reconnectTimer = null;

        try {
            socket = new WebSocket(url);
        } catch (error) {
            logger?.error("WebSocket setup failed", error);
            notifyState("disconnected");
            return;
        }

        socket.addEventListener("open", () => {
            logger?.info("WebSocket connection established");
            reconnectDelay = RECONNECT_DELAY_INITIAL_MS;
            reconnectAttempt = 0;
            notifyState("connected");
        });

        socket.addEventListener("message", (event) => {
            const size = typeof event.data === "string" ? event.data.length : undefined;
            telemetryStore.recordWsMessage(size);

            if (typeof event.data !== "string") {
                return;
            }

            try {
                handleMessage(JSON.parse(event.data));
            } catch (error) {
                logger?.warn("Failed to parse WebSocket payload", error);
            }
        });

        socket.addEventListener("close", (event) => {
            logger?.warn("WebSocket connection closed", {
                code: event.code,
                reason: event.reason || "No reason provided",
            });

            if (destroyed) return;

            reconnectAttempt++;
            notifyState("reconnecting", reconnectAttempt);
            logger?.info(`Reconnecting in ${reconnectDelay}ms (attempt ${reconnectAttempt})`);
            reconnectTimer = setTimeout(() => connect(), reconnectDelay);
            reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_DELAY_MAX_MS);
        });

        socket.addEventListener("error", (error) => {
            logger?.error("WebSocket encountered an error", error);
        });
    }

    connect();

    return {
        destroy() {
            destroyed = true;
            if (reconnectTimer !== null) {
                clearTimeout(reconnectTimer);
                reconnectTimer = null;
            }
            if (socket) {
                try {
                    socket.close();
                } catch {
                    // Ignore close errors during teardown.
                }
                socket = null;
            }
        },
    };
}
