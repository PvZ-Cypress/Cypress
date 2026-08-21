#pragma once

namespace Cypress::CFB27
{
	struct BridgeConfig;
	class BridgeLog;

	void LogImageDiscoverySummary(BridgeLog& log);
	bool PatchRedirectorHostnameTable(const BridgeConfig& config, BridgeLog& log);
	bool PatchRedirectorServiceNameTable(BridgeLog& log);
	bool InstallBearSslCertificateBypass(BridgeLog& log);
	void LogProtoSslRuntimeCodeBytes(BridgeLog& log);
	void LogRuntimeRedirectorReferences(BridgeLog& log);

	// Experimental execution-coverage probe (off by default; gated by
	// BridgeConfig::enableProtoSslVerifyProbe). Arms PAGE_GUARD over the known ProtoSSL
	// runtime-code regions and logs, on first execution, which of those functions run during
	// a live handshake. Purpose: pin the certificate-verify path the game actually uses when
	// the BearSSL end_chain hook never fires. Returns true if the handler and guards installed.
	bool InstallProtoSslVerifyProbe(BridgeLog& log);

	// Observes the ProtoSSL receive-state routine at RVA 0x16D1750. This is diagnostic only:
	// it never overrides a return value because the routine is not certificate verification.
	bool InstallCertVerifyHook(BridgeLog& log, bool force);

	// Hooks _ProtoSSLUpdate (RVA 0x16E1A40) and passively logs iState state[0x370]
	// transitions with a backtrace. It uses no debug registers or exceptions.
	bool InstallFailStateWatch(BridgeLog& log);
	std::size_t BoundedProtoSslReceiveLength(std::int64_t result, std::size_t requestedLength);
	std::size_t ProtoSslDiagnosticPreviewLimit();
}
