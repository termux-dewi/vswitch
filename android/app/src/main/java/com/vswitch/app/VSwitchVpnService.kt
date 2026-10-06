package com.vswitch.app

import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.FileInputStream
import java.io.FileOutputStream
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

class VSwitchVpnService : VpnService() {

    companion object {
        private const val TAG = "VSwitchVpn"
        const val EXTRA_HOST = "host"
        const val EXTRA_PORT = "port"
        const val EXTRA_SERVICE = "service"
        const val EXTRA_CA = "ca"
        const val EXTRA_SECURITY = "security"
        const val EXTRA_SNI = "sni"
        const val EXTRA_ALPN = "alpn"
        const val EXTRA_INSECURE = "insecure"
        const val EXTRA_ACTION = "action"
        const val ACTION_START = "start"
        const val ACTION_STOP = "stop"

        @Volatile var instance: VSwitchVpnService? = null
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var pfd: ParcelFileDescriptor? = null
    private var client: TunnelClient? = null
    private val bootQueue = LinkedBlockingQueue<ByteArray>()
    @Volatile private var bootstrapping = false
    @Volatile private var running = false

    private var myMac = ByteArray(6)
    private var myIp = ByteArray(4)
    private var gwIp = ByteArray(4)
    private var gwMac = ByteArray(6)

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.getStringExtra(EXTRA_ACTION)
        if (action == ACTION_STOP) { stopTunnel(); stopSelf(); return START_NOT_STICKY }
        val host = intent?.getStringExtra(EXTRA_HOST) ?: return START_NOT_STICKY
        val port = intent.getIntExtra(EXTRA_PORT, 50051)
        val service = intent.getStringExtra(EXTRA_SERVICE) ?: "android-client"
        val ca = intent.getByteArrayExtra(EXTRA_CA)
        val security = intent.getStringExtra(EXTRA_SECURITY) ?: "none"
        val sni = intent.getStringExtra(EXTRA_SNI) ?: ""
        val alpn = intent.getStringExtra(EXTRA_ALPN) ?: ""
        val insecure = intent.getBooleanExtra(EXTRA_INSECURE, false)
        instance = this
        scope.launch { runTunnel(host, port, service, ca, security, sni, alpn, insecure) }
        return START_STICKY
    }

    override fun onDestroy() { stopTunnel(); scope.cancel(); super.onDestroy() }

    override fun onRevoke() { stopTunnel() }

    private fun stopTunnel() {
        running = false
        try { client?.stop() } catch (_: Exception) {}
        client = null
        try { tunOut?.close() } catch (_: Exception) {}
        tunOut = null
        try { pfd?.close() } catch (_: Exception) {}
        pfd = null
        instance = null
    }

    private suspend fun runTunnel(host: String, port: Int, service: String, ca: ByteArray?, security: String, sni: String, alpn: String, insecure: Boolean) {
        deriveMac(service)
        bootstrapping = true
        val tunnel = TunnelClient(host, port, service, ca,
            security = security, sni = sni, alpn = alpn, allowInsecure = insecure,
            onFrame = { f ->
                if (bootstrapping) bootQueue.offer(f) else forwardIn(f)
            },
            onState = { ok, msg -> Log.i(TAG, "state=$ok $msg"); if (!ok && running) { stopTunnel(); stopSelf() } }
        )
        client = tunnel
        try { tunnel.start() } catch (e: Exception) { Log.e(TAG, "start", e); stopSelf(); return }

        val ok = withContext(Dispatchers.IO) { bootstrap(tunnel) }
        if (!ok) { Log.e(TAG, "bootstrap failed"); tunnel.stop(); stopSelf(); return }
        bootstrapping = false

        val iface = Builder()
            .addAddress(ipStr(myIp), 24)
            .addRoute("0.0.0.0", 0)
            .addDnsServer(ipStr(gwIp))
            .setSession("vSwitch")
            .setMtu(1400)
            .establish()
        if (iface == null) { Log.e(TAG, "establish failed"); tunnel.stop(); stopSelf(); return }
        pfd = iface
        running = true
        Log.i(TAG, "VPN up ip=${ipStr(myIp)} gw=${ipStr(gwIp)}")

        tunOut = FileOutputStream(iface.fileDescriptor)
        val inp = FileInputStream(iface.fileDescriptor)
        // device -> server
        scope.launch {
            val buf = ByteArray(2048)
            try {
                while (running) {
                    val n = inp.read(buf)
                    if (n < 20) continue
                    val f = ByteArray(14 + n)
                    System.arraycopy(gwMac, 0, f, 0, 6)
                    System.arraycopy(myMac, 0, f, 6, 6)
                    f[12] = 0x08; f[13] = 0x00
                    System.arraycopy(buf, 0, f, 14, n)
                    tunnel.send(f)
                }
            } catch (_: Exception) {}
        }
        keepAlive(tunnel)
    }

    private suspend fun keepAlive(tunnel: TunnelClient) {
        while (running) {
            kotlinx.coroutines.delay(1000)
        }
    }

    private var tunOut: FileOutputStream? = null

    private fun forwardIn(b: ByteArray) {
        if (b.size < 14) return
        val dst = b.copyOfRange(0, 6)
        if (!isBcast(dst) && !sameMac(dst, myMac)) return
        val eth = ((b[12].toInt() and 0xff) shl 8) or (b[13].toInt() and 0xff)
        if (eth == 0x0806 && b.size >= 42) {
            val op = ((b[20].toInt() and 0xff) shl 8) or (b[21].toInt() and 0xff)
            if (op == 1 && sameIp(b.copyOfRange(38, 42), myIp)) {
                val r = arpReply(myMac, myIp, b.copyOfRange(6, 12), b.copyOfRange(28, 32))
                client?.send(r)
            }
            return
        }
        if (eth != 0x0800) return
        val out = tunOut ?: return
        try { out.write(b, 14, b.size - 14) } catch (_: Exception) {}
    }

    // ---- DHCP/ARP bootstrap ----

    private suspend fun bootstrap(tunnel: TunnelClient): Boolean {
        val deadline = System.currentTimeMillis() + 10_000
        val discover = dhcpDiscover(myMac)
        tunnel.send(ethFrame(broadcast(), myMac, 0x0800, discover))
        while (System.currentTimeMillis() < deadline) {
            val frame = withContext(Dispatchers.IO) { bootQueue.poll(500, TimeUnit.MILLISECONDS) }
                ?: continue
            if (frame.size < 14) continue
            val eth = ((frame[12].toInt() and 0xff) shl 8) or (frame[13].toInt() and 0xff)
            if (eth != 0x0800) continue
            val ip = frame.copyOfRange(14, frame.size)
            if (ip.size < 20) continue
            if ((ip[9].toInt() and 0xff) != 17) continue
            val ipHdrLen = (ip[0].toInt() and 0x0f) * 4
            val udpData = ip.copyOfRange(ipHdrLen + 8, ip.size)
            if (udpData.size < 240) continue
            val msgType = parseDhcpOption(udpData, 53)
            if (msgType != null && msgType.isNotEmpty() && msgType[0] == 2.toByte()) {
                myIp = udpData.copyOfRange(16, 20)
                val gw = parseDhcpOption(udpData, 3)
                gwIp = gw ?: byteArrayOf(myIp[0], myIp[1], myIp[2], 1)
                val request = dhcpRequest(myMac, myIp, gwIp)
                tunnel.send(ethFrame(broadcast(), myMac, 0x0800, request))
                val ackDeadline = System.currentTimeMillis() + 5_000
                while (System.currentTimeMillis() < ackDeadline) {
                    val af = withContext(Dispatchers.IO) { bootQueue.poll(500, TimeUnit.MILLISECONDS) }
                        ?: continue
                    if (af.size < 14) continue
                    val ae = ((af[12].toInt() and 0xff) shl 8) or (af[13].toInt() and 0xff)
                    if (ae != 0x0800) continue
                    val ai = af.copyOfRange(14, af.size)
                    if (ai.size < 20) continue
                    if ((ai[9].toInt() and 0xff) != 17) continue
                    val ah = (ai[0].toInt() and 0x0f) * 4
                    val au = ai.copyOfRange(ah + 8, ai.size)
                    val at = parseDhcpOption(au, 53)
                    if (at != null && at.isNotEmpty() && at[0] == 5.toByte()) break
                }
                break
            }
        }
        if (myIp.all { it == 0.toByte() }) return false
        // ARP for gateway MAC
        val arpDeadline = System.currentTimeMillis() + 5_000
        tunnel.send(ethFrame(broadcast(), myMac, 0x0806, arpRequest(myMac, myIp, gwIp)))
        while (System.currentTimeMillis() < arpDeadline) {
            val frame = withContext(Dispatchers.IO) { bootQueue.poll(500, TimeUnit.MILLISECONDS) }
                ?: continue
            if (frame.size < 42) continue
            val eth = ((frame[12].toInt() and 0xff) shl 8) or (frame[13].toInt() and 0xff)
            if (eth != 0x0806) continue
            val op = ((frame[20].toInt() and 0xff) shl 8) or (frame[21].toInt() and 0xff)
            if (op == 2 && sameIp(frame.copyOfRange(28, 32), gwIp)) {
                gwMac = frame.copyOfRange(22, 28)
                return true
            }
        }
        gwMac = byteArrayOf(0x02, 0x00, 0x00, 0x00, 0x00, 0x01)
        return true
    }

    private fun deriveMac(service: String) {
        var hash = 0
        for (c in service) hash = hash * 31 + c.code
        myMac[0] = 0x02
        myMac[1] = ((hash shr 0) and 0xff).toByte()
        myMac[2] = ((hash shr 8) and 0xff).toByte()
        myMac[3] = ((hash shr 16) and 0xff).toByte()
        myMac[4] = ((hash shr 24) and 0xff).toByte()
        myMac[5] = ((hash shr 12) and 0xff).toByte()
    }

    private fun ipStr(b: ByteArray) = "${b[0].toInt() and 0xff}.${b[1].toInt() and 0xff}.${b[2].toInt() and 0xff}.${b[3].toInt() and 0xff}"
    private fun broadcast() = byteArrayOf(0xff.toByte(), 0xff.toByte(), 0xff.toByte(), 0xff.toByte(), 0xff.toByte(), 0xff.toByte())
    private fun sameMac(a: ByteArray, b: ByteArray): Boolean {
        if (a.size < 6 || b.size < 6) return false
        for (i in 0..5) if (a[i] != b[i]) return false
        return true
    }
    private fun sameIp(a: ByteArray, b: ByteArray): Boolean {
        if (a.size < 4 || b.size < 4) return false
        for (i in 0..3) if (a[i] != b[i]) return false
        return true
    }
    private fun isBcast(mac: ByteArray): Boolean {
        if (mac.size < 6) return false
        return mac.all { it == 0xff.toByte() }
    }

    private fun ethFrame(dst: ByteArray, src: ByteArray, etherType: Int, payload: ByteArray): ByteArray {
        val f = ByteArray(14 + payload.size)
        System.arraycopy(dst, 0, f, 0, 6)
        System.arraycopy(src, 0, f, 6, 6)
        f[12] = ((etherType shr 8) and 0xff).toByte()
        f[13] = (etherType and 0xff).toByte()
        System.arraycopy(payload, 0, f, 14, payload.size)
        return f
    }

    private fun arpRequest(srcMac: ByteArray, srcIp: ByteArray, targetIp: ByteArray): ByteArray {
        val arp = ByteArray(28)
        arp[0] = 0x00; arp[1] = 0x01
        arp[2] = 0x08; arp[3] = 0x00
        arp[4] = 0x06; arp[5] = 0x04
        arp[6] = 0x00; arp[7] = 0x01
        System.arraycopy(srcMac, 0, arp, 8, 6)
        System.arraycopy(srcIp, 0, arp, 14, 4)
        System.arraycopy(targetIp, 0, arp, 24, 4)
        return arp
    }

    private fun arpReply(srcMac: ByteArray, srcIp: ByteArray, dstMac: ByteArray, dstIp: ByteArray): ByteArray {
        val arp = ByteArray(28)
        arp[0] = 0x00; arp[1] = 0x01
        arp[2] = 0x08; arp[3] = 0x00
        arp[4] = 0x06; arp[5] = 0x04
        arp[6] = 0x00; arp[7] = 0x02
        System.arraycopy(srcMac, 0, arp, 8, 6)
        System.arraycopy(srcIp, 0, arp, 14, 4)
        System.arraycopy(dstMac, 0, arp, 18, 6)
        System.arraycopy(dstIp, 0, arp, 24, 4)
        return arp
    }

    private fun dhcpDiscover(mac: ByteArray): ByteArray = dhcpMessage(mac, 1, null, null)
    private fun dhcpRequest(mac: ByteArray, reqIp: ByteArray, srvIp: ByteArray): ByteArray = dhcpMessage(mac, 3, reqIp, srvIp)

    private fun dhcpMessage(mac: ByteArray, msgType: Int, reqIp: ByteArray?, srvIp: ByteArray?): ByteArray {
        val dhcp = ByteArray(300)
        dhcp[0] = 1; dhcp[1] = 1; dhcp[2] = 6
        dhcp[4] = 0x12; dhcp[5] = 0x34; dhcp[6] = 0x56; dhcp[7] = 0x78
        System.arraycopy(mac, 0, dhcp, 28, 6)
        dhcp[236] = 0x63.toByte(); dhcp[237] = 0x82.toByte()
        dhcp[238] = 0x53.toByte(); dhcp[239] = 0x63.toByte()
        var off = 240
        dhcp[off++] = 53.toByte(); dhcp[off++] = 1; dhcp[off++] = msgType.toByte()
        if (reqIp != null) { dhcp[off++] = 50.toByte(); dhcp[off++] = 4; System.arraycopy(reqIp, 0, dhcp, off, 4); off += 4 }
        if (srvIp != null) { dhcp[off++] = 54.toByte(); dhcp[off++] = 4; System.arraycopy(srvIp, 0, dhcp, off, 4); off += 4 }
        dhcp[off++] = 0xff.toByte()
        val dhcpLen = off
        val udpLen = 8 + dhcpLen
        val udp = ByteArray(udpLen)
        udp[0] = 0x00; udp[1] = 0x44
        udp[2] = 0x00; udp[3] = 0x43
        udp[4] = ((udpLen shr 8) and 0xff).toByte(); udp[5] = (udpLen and 0xff).toByte()
        System.arraycopy(dhcp, 0, udp, 8, dhcpLen)
        val ipTotalLen = 20 + udpLen
        val ip = ByteArray(ipTotalLen)
        ip[0] = 0x45
        ip[2] = ((ipTotalLen shr 8) and 0xff).toByte(); ip[3] = (ipTotalLen and 0xff).toByte()
        ip[8] = 0x80.toByte(); ip[9] = 17
        ip[16] = 0xff.toByte(); ip[17] = 0xff.toByte(); ip[18] = 0xff.toByte(); ip[19] = 0xff.toByte()
        var sum = 0
        for (i in 0 until 20 step 2) sum += ((ip[i].toInt() and 0xff) shl 8) or (ip[i + 1].toInt() and 0xff)
        while ((sum shr 16) != 0) sum = (sum and 0xffff) + (sum shr 16)
        val csum = sum.inv() and 0xffff
        ip[10] = ((csum shr 8) and 0xff).toByte(); ip[11] = (csum and 0xff).toByte()
        System.arraycopy(udp, 0, ip, 20, udpLen)
        return ip
    }

    private fun parseDhcpOption(udp: ByteArray, optionCode: Int): ByteArray? {
        if (udp.size < 248) return null
        var off = 248
        while (off < udp.size) {
            val code = udp[off].toInt() and 0xff
            if (code == 0xff) break
            if (code == 0) { off++; continue }
            off++
            if (off >= udp.size) break
            val len = udp[off].toInt() and 0xff
            off++
            if (code == optionCode) {
                if (off + len > udp.size) return null
                return udp.copyOfRange(off, off + len)
            }
            off += len
        }
        return null
    }
}
