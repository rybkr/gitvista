export async function startBackend({ onDelta, onStatus, logger }) {
    await loadRepositoryMetadata(logger);
    return openWebSocket({ onDelta, onStatus, logger });
}

async function loadRepositoryMetadata(logger) {
    logger?.info("Requesting repository metadata");
    try {
        const response = await fetch("/api/repository");
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        await response.json();
        logger?.info("Repository metadata loaded");
    } catch (error) {
        logger?.error("Failed to load repository metadata", error);
    }
}

function openWebSocket({ onDelta, onStatus, logger }) {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${protocol}://${window.location.host}/api/ws`;
    logger?.info("Opening WebSocket connection", url);

    try {
        const socket = new WebSocket(url);

        socket.addEventListener("open", () => {
            logger?.info("WebSocket connection established");
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
            } catch (error) {
                logger?.warn("Failed to parse WebSocket payload", error);
            }
        });

        socket.addEventListener("close", (event) => {
            logger?.warn("WebSocket connection closed", {
                code: event.code,
                reason: event.reason || "No reason provided",
            });
        });

        socket.addEventListener("error", (error) => {
            logger?.error("WebSocket encountered an error", error);
        });

        return socket;
    } catch (error) {
        logger?.error("WebSocket setup failed", error);
        return null;
    }
}

