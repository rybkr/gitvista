import { apiUrl, wsUrl } from "./apiBase.js";

// Reconnection backoff constants
const RECONNECT_DELAY_INITIAL_MS = 1000;
const RECONNECT_DELAY_MAX_MS = 30000;

export async function startBackend({ onDelta, onStatus, onHead, onRepoMetadata, onConnectionStateChange, logger }) {
    await loadRepositoryMetadata(logger, onRepoMetadata);
    return openWebSocket({ onDelta, onStatus, onHead, onRepoMetadata, onConnectionStateChange, logger });
}

async function loadRepositoryMetadata(logger, onRepoMetadata) {
    logger?.info("Requesting repository metadata");
    try {
        const response = await fetch(apiUrl("/repository"));
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        const metadata = await response.json();
        logger?.info("Repository metadata loaded");
        onRepoMetadata?.(metadata);
    } catch (error) {
        logger?.error("Failed to load repository metadata", error);
    }
}

function openWebSocket({ onDelta, onStatus, onHead, onRepoMetadata, onConnectionStateChange, logger }) {
    const url = wsUrl();
    logger?.info("Opening WebSocket connection", url);

    let reconnectDelay = RECONNECT_DELAY_INITIAL_MS;
    let destroyed = false;

    function notifyState(state) {
        onConnectionStateChange?.(state);
    }

    function connect() {
        if (destroyed) return;

        let socket;
        try {
            socket = new WebSocket(url);
        } catch (error) {
            logger?.error("WebSocket setup failed", error);
            notifyState("disconnected");
            return;
        }

        socket.addEventListener("open", () => {
            logger?.info("WebSocket connection established");
            const isReconnect = reconnectDelay > RECONNECT_DELAY_INITIAL_MS;
            reconnectDelay = RECONNECT_DELAY_INITIAL_MS;
            notifyState("connected");
            // Re-fetch metadata on reconnect so the info bar reflects any changes
            // (new branches, changed HEAD, etc.) that occurred while disconnected.
            if (isReconnect) {
                loadRepositoryMetadata(logger, onRepoMetadata);
            }
        });

        socket.addEventListener("message", (event) => {
            const size = typeof event.data === "string" ? event.data.length : undefined;
            if (size !== undefined) {
                logger?.info("WebSocket message received", { size });
            } else {
                logger?.info("WebSocket message received");
            }

            if (typeof event.data !== "string") {
                return;
            }

            try {
                const payload = JSON.parse(event.data);
                if (payload?.delta) {
                    onDelta?.(payload.delta);
                }
                if (payload?.status) {
                    onStatus?.(payload.status);
                }
                if (payload?.head) {
                    onHead?.(payload.head);
                }
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

            // Reconnect with exponential backoff regardless of close code.
            notifyState("reconnecting");
            logger?.info(`Reconnecting in ${reconnectDelay}ms`);

            setTimeout(() => connect(), reconnectDelay);

            reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_DELAY_MAX_MS);
        });

        socket.addEventListener("error", (error) => {
            logger?.error("WebSocket encountered an error", error);
            // The close event fires after error, so reconnection is handled there.
        });
    }

    connect();
}

