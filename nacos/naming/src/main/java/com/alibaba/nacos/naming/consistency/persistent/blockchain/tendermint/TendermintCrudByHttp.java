package com.alibaba.nacos.naming.consistency.persistent.blockchain.tendermint;

import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.alibaba.nacos.naming.consistency.persistent.blockchain.BlockchainCrud;
import com.alibaba.nacos.naming.misc.Loggers;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Primary;
import org.springframework.stereotype.Component;

import java.io.BufferedReader;
import java.io.OutputStream;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.UUID;


/**
 * 实现了BlockchainCrud（方法名仍叫fabricPut、fabricQueryByKey、fabricDelete、fabricQueryAllNamingData），但是实际上是通过HTTP调用wfConsensusBridge的ABCI应用
 * rpcUrl：tendermint.rpc.url 或环境变量 TENDERMINT_RPC_URL，默认 http://127.0.0.1:26657（Tendermint 的 JSON-RPC）。
 */

/**
 * Stores Nacos naming data through the wfConsensusBridge ABCI application.
 */
@Primary
@Component
public class TendermintCrudByHttp implements BlockchainCrud {

    @Value("${tendermint.rpc.url:${TENDERMINT_RPC_URL:http://127.0.0.1:26657}}")
    private String rpcUrl;

    @Override
    public String fabricPut(String key, String value) throws Exception {
        JSONObject payload = new JSONObject();
        payload.put("key", key);
        payload.put("value", value);
        broadcast("nacos_put", payload);
        return "ok";
    }

    @Override
    public String fabricQueryByKey(String key) throws Exception {
        return query("/nacos/key", key);
    }

    @Override
    public String fabricDelete(String key) throws Exception {
        JSONObject payload = new JSONObject();
        payload.put("key", key);
        broadcast("nacos_delete", payload);
        return "ok";
    }

    @Override
    public String fabricQueryAllNamingData() throws Exception {
        return query("/nacos/prefix", "com.alibaba.nacos.naming");
    }

    private void broadcast(String type, JSONObject payload) throws Exception {
        JSONObject tx = new JSONObject();
        tx.put("txId", UUID.randomUUID().toString());
        tx.put("type", type);
        tx.put("payload", payload);

        JSONObject params = new JSONObject();
        params.put("tx", Base64.getEncoder().encodeToString(JSON.toJSONString(tx).getBytes(StandardCharsets.UTF_8)));
        JSONObject result = rpc("broadcast_tx_commit", params).getJSONObject("result");
        ensureTxResultOk(result, "check_tx");
        ensureTxResultOk(result, "deliver_tx");
        ensureTxResultOk(result, "tx_result");
    }

    private String query(String path, String data) throws Exception {
        JSONObject params = new JSONObject();
        params.put("path", path);
        params.put("data", bytesToHex(data.getBytes(StandardCharsets.UTF_8)));
        params.put("prove", false);

        JSONObject response = rpc("abci_query", params).getJSONObject("result").getJSONObject("response");
        int code = response.getIntValue("code");
        if (code != 0) {
            throw new IllegalStateException(response.getString("log"));
        }
        String value = response.getString("value");
        if (value == null || value.trim().isEmpty()) {
            return "";
        }
        return new String(Base64.getDecoder().decode(value), StandardCharsets.UTF_8);
    }

    private JSONObject rpc(String method, JSONObject params) throws Exception {
        JSONObject request = new JSONObject();
        request.put("jsonrpc", "2.0");
        request.put("id", "nacos");
        request.put("method", method);
        request.put("params", params);

        HttpURLConnection connection = (HttpURLConnection) new URL(trimRight(rpcUrl) + "/").openConnection();
        connection.setRequestMethod("POST");
        connection.setDoOutput(true);
        connection.setConnectTimeout(5000);
        connection.setReadTimeout(30000);
        connection.setRequestProperty("Content-Type", "application/json");

        byte[] body = JSON.toJSONString(request).getBytes(StandardCharsets.UTF_8);
        try (OutputStream out = connection.getOutputStream()) {
            out.write(body);
        }

        String raw = readResponse(connection);
        JSONObject response = JSON.parseObject(raw);
        if (response.containsKey("error")) {
            throw new IllegalStateException(response.getJSONObject("error").toJSONString());
        }
        Loggers.RAFT.info("tendermint rpc {} response: {}", method, raw);
        return response;
    }

    private String readResponse(HttpURLConnection connection) throws Exception {
        int status = connection.getResponseCode();
        BufferedReader reader = new BufferedReader(new InputStreamReader(
                status >= 300 ? connection.getErrorStream() : connection.getInputStream(), StandardCharsets.UTF_8));
        StringBuilder builder = new StringBuilder();
        String line;
        while ((line = reader.readLine()) != null) {
            builder.append(line);
        }
        if (status >= 300) {
            throw new IllegalStateException("tendermint http " + status + ": " + builder);
        }
        return builder.toString();
    }

    private void ensureTxResultOk(JSONObject result, String field) {
        JSONObject txResult = result.getJSONObject(field);
        if (txResult == null) {
            return;
        }
        int code = txResult.getIntValue("code");
        if (code != 0) {
            throw new IllegalStateException(field + " failed: " + txResult.getString("log"));
        }
    }

    private String trimRight(String value) {
        if (value == null || value.trim().isEmpty()) {
            return "http://127.0.0.1:26657";
        }
        String out = value.trim();
        while (out.endsWith("/")) {
            out = out.substring(0, out.length() - 1);
        }
        return out;
    }

    private String bytesToHex(byte[] bytes) {
        char[] hex = new char[bytes.length * 2];
        char[] digits = "0123456789ABCDEF".toCharArray();
        for (int i = 0; i < bytes.length; i++) {
            int value = bytes[i] & 0xFF;
            hex[i * 2] = digits[value >>> 4];
            hex[i * 2 + 1] = digits[value & 0x0F];
        }
        return new String(hex);
    }
}
