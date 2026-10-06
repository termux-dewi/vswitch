package com.vswitch.app

import com.vswitch.app.proto.Frame
import com.vswitch.app.proto.TunnelServiceGrpc
import io.grpc.ManagedChannel
import io.grpc.Metadata
import io.grpc.Status
import io.grpc.okhttp.OkHttpChannelBuilder
import io.grpc.stub.MetadataUtils
import io.grpc.stub.StreamObserver
import java.io.ByteArrayInputStream
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager

class TunnelClient(
    private val host: String,
    private val port: Int,
    private val serviceName: String,
    private val caCertPem: ByteArray?,
    private val security: String = "none",
    private val sni: String = "",
    private val alpn: String = "",
    private val allowInsecure: Boolean = false,
    private val onFrame: (ByteArray) -> Unit,
    private val onState: (Boolean, String) -> Unit
) {
    private var channel: ManagedChannel? = null
    private var requestObserver: StreamObserver<Frame>? = null

    @Volatile
    var connected = false
        private set

    fun start() {
        val builder = OkHttpChannelBuilder.forAddress(host, port)
        if (security == "tls") {
            builder.sslSocketFactory(buildSslContext().socketFactory)
            if (sni.isNotEmpty()) {
                builder.overrideAuthority(sni)
            }
        } else {
            builder.usePlaintext()
        }
        builder.keepAliveTime(10, TimeUnit.SECONDS)
            .keepAliveTimeout(5, TimeUnit.SECONDS)

        val ch = builder.build()
        channel = ch

        val header = Metadata.Key.of("x-service-name", Metadata.ASCII_STRING_MARSHALLER)
        val md = Metadata()
        md.put(header, serviceName)
        val stub = TunnelServiceGrpc.newStub(ch)
            .withInterceptors(MetadataUtils.newAttachHeadersInterceptor(md))

        requestObserver = stub.stream(object : StreamObserver<Frame> {
            override fun onNext(value: Frame) {
                onFrame(value.payload.toByteArray())
            }

            override fun onError(t: Throwable) {
                connected = false
                onState(false, Status.fromThrowable(t).toString())
            }

            override fun onCompleted() {
                connected = false
                onState(false, "stream closed")
            }
        })
        connected = true
        onState(true, "connected")
    }

    fun send(frame: ByteArray) {
        if (!connected) return
        try {
            requestObserver?.onNext(
                Frame.newBuilder().setPayload(com.google.protobuf.ByteString.copyFrom(frame)).build()
            )
        } catch (_: Exception) {
        }
    }

    fun stop() {
        connected = false
        try {
            requestObserver?.onCompleted()
        } catch (_: Exception) {
        }
        try {
            channel?.shutdownNow()
        } catch (_: Exception) {
        }
        channel = null
        requestObserver = null
    }

    private fun buildSslContext(): SSLContext {
        val ctx = SSLContext.getInstance("TLS")
        when {
            allowInsecure -> {
                val trustAll = object : X509TrustManager {
                    override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) {}
                    override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) {}
                    override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
                }
                ctx.init(null, arrayOf(trustAll), null)
            }
            caCertPem != null -> {
                val cf = CertificateFactory.getInstance("X.509")
                val cert = cf.generateCertificate(ByteArrayInputStream(caCertPem))
                val ks = KeyStore.getInstance(KeyStore.getDefaultType())
                ks.load(null, null)
                ks.setCertificateEntry("ca", cert)
                val tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
                tmf.init(ks)
                ctx.init(null, tmf.trustManagers, null)
            }
            else -> {
                ctx.init(null, null, null)
            }
        }
        return ctx
    }
}
