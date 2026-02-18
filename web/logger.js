const LOG_PREFIX = "[GitVista]";

function log(level, message, detail) {
    const time = new Date().toISOString();
    const fn = level === "ERROR" ? console.error : level === "WARN" ? console.warn : console.log;
    if (detail !== undefined) {
        fn(`${LOG_PREFIX} ${time} [${level}] ${message}`, detail);
        return;
    }
    fn(`${LOG_PREFIX} ${time} [${level}] ${message}`);
}

export const logger = {
    info(message, detail) {
        log("INFO", message, detail);
    },
    warn(message, detail) {
        log("WARN", message, detail);
    },
    error(message, detail) {
        log("ERROR", message, detail);
    },
};

