package org.activiti.engine.impl.db.redis;

public class RedisKeyBuilder {
    private static final String DEFAULT_PREFIX = "wf";
    private static final String prefix = initPrefix();

    private static String initPrefix() {
        String env = System.getenv("WF_REDIS_KEY_PREFIX");
        if (env == null || env.trim().isEmpty()) {
            return DEFAULT_PREFIX;
        }
        return env.trim();
    }

    public static String entityKey(String rawKey) {
        return prefix + ":entity:" + rawKey;
    }

    public static String deployKey(String deploymentName) {
        return prefix + ":deploy:" + deploymentName;
    }

    public static String orgKey(String orgName) {
        return prefix + ":org:" + orgName;
    }

    public static String allocKey(String oid) {
        return prefix + ":alloc:" + oid;
    }

    public static String prefix() {
        return prefix;
    }
}
