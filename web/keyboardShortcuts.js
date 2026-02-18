/**
 * @fileoverview Global keyboard shortcut system for GitVista.
 * Registers a single keydown listener and dispatches named actions via callbacks.
 * Supports single-key shortcuts and the two-key G→H sequence for jump-to-HEAD.
 */

// Maximum time (ms) allowed between G and H for the G→H sequence to register.
const SEQUENCE_TIMEOUT_MS = 500;

/**
 * Creates and installs the global keyboard shortcut handler.
 *
 * @param {{
 *   onJumpToHead?: () => void,
 *   onFocusSearch?: () => void,
 *   onToggleHelp?: () => void,
 *   onDismiss?: () => void,
 *   onNavigatePrev?: () => void,
 *   onNavigateNext?: () => void,
 * }} callbacks Named action callbacks. Any omitted callback is silently ignored.
 * @returns {{ destroy(): void }} Handle to remove the listener and cancel any pending timer.
 */
export function createKeyboardShortcuts({
    onJumpToHead,
    onFocusSearch,
    onToggleHelp,
    onDismiss,
    onNavigatePrev,
    onNavigateNext,
} = {}) {
    // True when the user has pressed G and is within the timeout window for H.
    let awaitingH = false;
    let sequenceTimer = null;

    /**
     * Returns true when keyboard focus is inside an input element where
     * we must not intercept printable keys to avoid breaking normal typing.
     *
     * @returns {boolean}
     */
    function isTypingFocused() {
        const active = document.activeElement;
        if (!active) {
            return false;
        }
        const tag = active.tagName;
        return tag === "INPUT" || tag === "TEXTAREA" || active.isContentEditable;
    }

    function clearSequence() {
        awaitingH = false;
        if (sequenceTimer !== null) {
            clearTimeout(sequenceTimer);
            sequenceTimer = null;
        }
    }

    /**
     * Central keydown handler attached to the document.
     *
     * @param {KeyboardEvent} event
     */
    function handleKeyDown(event) {
        // Never shadow browser/OS shortcuts that use modifier keys.
        if (event.ctrlKey || event.metaKey || event.altKey) {
            clearSequence();
            return;
        }

        // Escape is special: it fires even while an input is focused so the user
        // can always dismiss overlays without first clicking away.
        if (event.key === "Escape") {
            onDismiss?.();
            clearSequence();
            return;
        }

        // All other shortcuts are suppressed when the user is typing in a field.
        if (isTypingFocused()) {
            clearSequence();
            return;
        }

        // Resolve the pending G→H two-key sequence.
        if (awaitingH) {
            clearSequence();
            if (event.key === "h" || event.key === "H") {
                event.preventDefault();
                onJumpToHead?.();
                return;
            }
            // Any other key cancels the sequence; fall through to single-key handling.
        }

        switch (event.key) {
            case "g":
            case "G":
                // Begin the G→H sequence; a timer clears it if H doesn't arrive in time.
                event.preventDefault();
                awaitingH = true;
                sequenceTimer = setTimeout(clearSequence, SEQUENCE_TIMEOUT_MS);
                break;

            case "/":
                event.preventDefault();
                onFocusSearch?.();
                break;

            case "?":
                event.preventDefault();
                onToggleHelp?.();
                break;

            // J = navigate to next (newer) commit; K = navigate to prev (older) commit.
            // This mirrors the vi-style navigation convention used in many code tools.
            case "j":
            case "J":
                event.preventDefault();
                onNavigateNext?.();
                break;

            case "k":
            case "K":
                event.preventDefault();
                onNavigatePrev?.();
                break;

            default:
                break;
        }
    }

    document.addEventListener("keydown", handleKeyDown);

    return {
        /** Removes the global keydown listener and cancels any pending sequence timer. */
        destroy() {
            document.removeEventListener("keydown", handleKeyDown);
            clearSequence();
        },
    };
}
