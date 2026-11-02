package dev.nawa.music

import org.json.JSONArray
import org.json.JSONObject

object CoreBridge {
    init { System.loadLibrary("nawacore") }
    private external fun nativeStart(root: ByteArray, config: ByteArray, secret: ByteArray): ByteArray
    private external fun nativeRpc(method: ByteArray, args: ByteArray): ByteArray
    private external fun nativePoll(): ByteArray
    private external fun nativeComplete(id: ByteArray, data: ByteArray, error: ByteArray)
    var address = ""
    var token = ""
    fun start(root: String, config: JSONObject, secret: String) {
        val r = JSONObject(String(nativeStart(root.toByteArray(), config.toString().toByteArray(), secret.toByteArray()), Charsets.UTF_8))
        check(r.optBoolean("ok")) { r.optString("error") }
        address = r.getString("address"); token = r.getString("token")
    }
    fun raw(method: String, args: JSONArray = JSONArray()): ByteArray = nativeRpc(method.toByteArray(), args.toString().toByteArray())
    fun call(method: String, args: JSONArray = JSONArray()): Any? {
        val r = JSONObject(String(raw(method, args), Charsets.UTF_8))
        check(r.optBoolean("ok")) { r.optString("error") }
        return r.opt("result").takeUnless { it == JSONObject.NULL }
    }
    fun poll(): JSONObject? = String(nativePoll(), Charsets.UTF_8).let { if(it == "null") null else JSONObject(it) }
    fun complete(id: String, data: Any? = null, error: String = "") = nativeComplete(id.toByteArray(), (data?.toString() ?: "null").toByteArray(), error.toByteArray())
    fun mediaUrl(url: String) = when {
        url.startsWith("https://appassets.androidplatform.net/") -> address + "/" + url.substringAfter(".net/")
        url.startsWith("/") -> address + url
        else -> url
    }
}
