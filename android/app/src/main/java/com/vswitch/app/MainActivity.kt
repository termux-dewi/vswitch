package com.vswitch.app

import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity

class MainActivity : AppCompatActivity() {

    private lateinit var etHost: EditText
    private lateinit var etPort: EditText
    private lateinit var etService: EditText
    private lateinit var spnSecurity: Spinner
    private lateinit var etSni: EditText
    private lateinit var etAlpn: EditText
    private lateinit var cbInsecure: CheckBox
    private lateinit var btnCa: Button
    private lateinit var tvCa: TextView
    private lateinit var btnConnect: Button
    private lateinit var btnDisconnect: Button
    private lateinit var tvStatus: TextView

    private var caCertBytes: ByteArray? = null

    private val vpnPermissionLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
            if (result.resultCode == RESULT_OK) {
                startVpnService()
            } else {
                Toast.makeText(this, "VPN permission denied", Toast.LENGTH_SHORT).show()
            }
        }

    private val caFilePicker =
        registerForActivityResult(ActivityResultContracts.GetContent()) { uri ->
            if (uri != null) {
                try {
                    val bytes = contentResolver.openInputStream(uri)?.readBytes()
                    if (bytes != null && bytes.isNotEmpty()) {
                        caCertBytes = bytes
                        val name = uri.lastPathSegment ?: "cert"
                        tvCa.text = "CA: $name (${bytes.size} bytes)"
                    }
                } catch (e: Exception) {
                    Toast.makeText(this, "Failed to read cert: ${e.message}", Toast.LENGTH_SHORT).show()
                }
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        etHost = findViewById(R.id.etHost)
        etPort = findViewById(R.id.etPort)
        etService = findViewById(R.id.etService)
        spnSecurity = findViewById(R.id.spnSecurity)
        etSni = findViewById(R.id.etSni)
        etAlpn = findViewById(R.id.etAlpn)
        cbInsecure = findViewById(R.id.cbInsecure)
        btnCa = findViewById(R.id.btnCa)
        tvCa = findViewById(R.id.tvCa)
        btnConnect = findViewById(R.id.btnConnect)
        btnDisconnect = findViewById(R.id.btnDisconnect)
        tvStatus = findViewById(R.id.tvStatus)

        spnSecurity.adapter = ArrayAdapter(
            this, android.R.layout.simple_spinner_dropdown_item, arrayOf("none", "tls")
        )

        btnCa.setOnClickListener {
            caFilePicker.launch("*/*")
        }

        btnConnect.setOnClickListener {
            val host = etHost.text.toString().trim()
            val portStr = etPort.text.toString().trim()
            if (host.isEmpty() || portStr.isEmpty()) {
                Toast.makeText(this, "Host dan port wajib diisi", Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }
            val intent = VpnService.prepare(this)
            if (intent != null) {
                vpnPermissionLauncher.launch(intent)
            } else {
                startVpnService()
            }
        }

        btnDisconnect.setOnClickListener {
            stopVpnService()
        }
    }

    private fun startVpnService() {
        val host = etHost.text.toString().trim()
        val port = etPort.text.toString().trim().toIntOrNull() ?: 50051
        val service = etService.text.toString().trim().ifEmpty { "android-client" }
        val security = spnSecurity.selectedItem?.toString() ?: "none"
        val sni = etSni.text.toString().trim()
        val alpn = etAlpn.text.toString().trim()
        val insecure = cbInsecure.isChecked

        val intent = Intent(this, VSwitchVpnService::class.java).apply {
            putExtra(VSwitchVpnService.EXTRA_ACTION, VSwitchVpnService.ACTION_START)
            putExtra(VSwitchVpnService.EXTRA_HOST, host)
            putExtra(VSwitchVpnService.EXTRA_PORT, port)
            putExtra(VSwitchVpnService.EXTRA_SERVICE, service)
            putExtra(VSwitchVpnService.EXTRA_CA, caCertBytes)
            putExtra(VSwitchVpnService.EXTRA_SECURITY, security)
            putExtra(VSwitchVpnService.EXTRA_SNI, sni)
            putExtra(VSwitchVpnService.EXTRA_ALPN, alpn)
            putExtra(VSwitchVpnService.EXTRA_INSECURE, insecure)
        }
        startService(intent)

        btnConnect.isEnabled = false
        btnDisconnect.isEnabled = true
        tvStatus.text = "Status: connecting to $host:$port ..."
    }

    private fun stopVpnService() {
        val intent = Intent(this, VSwitchVpnService::class.java).apply {
            putExtra(VSwitchVpnService.EXTRA_ACTION, VSwitchVpnService.ACTION_STOP)
        }
        startService(intent)

        btnConnect.isEnabled = true
        btnDisconnect.isEnabled = false
        tvStatus.text = "Status: disconnected"
    }
}
