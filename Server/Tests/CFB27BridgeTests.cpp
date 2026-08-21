#include <CFB27/BridgeConfig.h>
#include <CFB27/BridgeLog.h>
#include <CFB27/MemoryDiscovery.h>
#include <CFB27/PatternScanner.h>
#include <CFB27/RedirectorHook.h>

#include <Windows.h>

#include <cstdint>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <string>
#include <vector>

namespace
{
	int s_failures = 0;

	void Check(bool condition, const char* message)
	{
		if (condition)
			return;
		std::cerr << "FAIL: " << message << '\n';
		++s_failures;
	}

	void TestEnvironmentConfig()
	{
		_putenv_s("CYPRESS_CFB27_BLAZE_HOST", "127.0.0.2");
		_putenv_s("CYPRESS_CFB27_BLAZE_PORT", "29000");
		_putenv_s("CYPRESS_CFB27_PROFILE", "TestProfile");
		_putenv_s("CYPRESS_CFB27_RUN_DIR", "C:\\tmp\\cfb27-test");
		_putenv_s("CYPRESS_CFB27_ENABLE_CANDIDATE_ENDPOINT_REDIRECTS", "0");
		_putenv_s("CYPRESS_CFB27_ENABLE_REDIRECTOR_NETWORK_REDIRECT", "0");

		auto config = Cypress::CFB27::BridgeConfig::FromEnvironment();
		Check(config.blazeHost == "127.0.0.2", "bridge host should come from the environment");
		Check(config.blazePort == 29000, "bridge port should come from the environment");
		Check(config.profile == "TestProfile", "profile should come from the environment");
		Check(config.runDirectory == "C:\\tmp\\cfb27-test", "run directory should come from the environment");
		Check(!config.enableCandidateEndpointRedirects, "candidate endpoint redirects should default off");
		Check(!config.enableRedirectorNetworkRedirect, "redirector network redirect should support observation mode");

		_putenv_s("CYPRESS_CFB27_ENABLE_CANDIDATE_ENDPOINT_REDIRECTS", "1");
		config = Cypress::CFB27::BridgeConfig::FromEnvironment();
		Check(config.enableCandidateEndpointRedirects, "candidate endpoint redirects should be explicit opt-in");
		_putenv_s("CYPRESS_CFB27_ENABLE_REDIRECTOR_NETWORK_REDIRECT", "1");
		config = Cypress::CFB27::BridgeConfig::FromEnvironment();
		Check(config.enableRedirectorNetworkRedirect, "redirector network redirect should be explicit opt-in");
	}

	void TestSHA256()
	{
		char tempPath[MAX_PATH] = {};
		char tempFile[MAX_PATH] = {};
		GetTempPathA(MAX_PATH, tempPath);
		GetTempFileNameA(tempPath, "cfb", 0, tempFile);
		{
			std::ofstream output(tempFile, std::ios::binary | std::ios::trunc);
			output << "abc";
		}
		const auto digest = Cypress::CFB27::SHA256FileHex(tempFile);
		DeleteFileA(tempFile);
		Check(
			digest == "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD",
			"SHA-256 should match the standard abc vector");
	}

	void TestUniquePattern()
	{
		const std::vector<std::uint8_t> data{0x10, 0x20, 0x30, 0x40, 0x20, 0x31, 0x40};
		const std::vector<int> pattern{0x20, -1, 0x40};
		Check(
			Cypress::CFB27::FindUniquePattern(data.data(), data.size(), pattern) ==
				Cypress::CFB27::PatternAmbiguous,
			"wildcard pattern should report ambiguity");

		const std::vector<int> unique{0x10, 0x20, -1};
		Check(
			Cypress::CFB27::FindUniquePattern(data.data(), data.size(), unique) == 0,
			"unique pattern should report its offset");

		const std::vector<int> missing{0x99};
		Check(
			Cypress::CFB27::FindUniquePattern(data.data(), data.size(), missing) ==
				Cypress::CFB27::PatternNotFound,
			"missing pattern should be reported");
	}

	void TestBridgeLog()
	{
		char tempPath[MAX_PATH] = {};
		GetTempPathA(MAX_PATH, tempPath);
		const std::string runDirectory = std::string(tempPath) + "cypress-cfb27-bridge-test";
		Cypress::CFB27::BridgeLog log;
		Check(log.Open(runDirectory), "bridge log should open in the run directory");
		log.Write("test-line");

		std::ifstream input(runDirectory + "\\cfb27-bridge.log", std::ios::binary);
		const std::string contents(
			(std::istreambuf_iterator<char>(input)),
			std::istreambuf_iterator<char>());
		Check(contents.find("test-line") != std::string::npos, "bridge log should contain the written line");
	}

	void TestRedirectorMatching()
	{
		Check(
			Cypress::CFB27::IsBlazeRedirectorHost("spring25.client.blazeredirector.ea.com"),
			"production Blaze redirector should match");
		Check(
			Cypress::CFB27::IsBlazeRedirectorHost("SPRING25.CLIENT.TEST.BLAZEREDIRECTOR.EA.COM"),
			"Blaze redirector matching should ignore case");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("accounts.ea.com"),
			"unrelated EA host should not match");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("gcs.ea.com"),
			"non-Blaze content service should not match");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("collector.errors.ea.com"),
			"telemetry collector should not match");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("a-collector.errors.ea.com"),
			"alternate telemetry collector should not match");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("freeform-river.data.ea.com"),
			"telemetry data service should not match");
		Check(
			!Cypress::CFB27::IsBlazeRedirectorHost("update.layer.ea.com"),
			"update service should not match");
		Check(
			Cypress::CFB27::IsKnownRedirectorIPv4(0xA6751733),
			"166.117.23.51 should match the known production redirector");
		Check(
			!Cypress::CFB27::IsKnownRedirectorIPv4(0x7F000001),
			"loopback should not be treated as an EA redirector");
	}

	void TestCandidateEndpointParsing()
	{
		const std::string json = R"json({
			"candidates": [
				{ "address": "5.60.70.149", "port": 15103 },
				{ "address": "5.60.70.149", "port": 15103 },
				{ "address": "52.211.83.198", "port": 443 }
			]
		})json";
		const auto endpoints = Cypress::CFB27::ParseCandidateEndpointsJson(json);
		Check(endpoints.size() == 2, "candidate endpoint parser should load unique IP:port pairs");
		if (!endpoints.empty())
		{
			Check(
				endpoints[0].hostOrderAddress == 0x053C4695 && endpoints[0].port == 15103,
				"candidate endpoint parser should preserve host-order IPv4 and port");
		}
	}

	void TestRedirectedSocketDiagnosticTracking()
	{
		constexpr std::uintptr_t socket = 0xC0FFEE;
		Check(
			!Cypress::CFB27::IsRedirectedSocketForDiagnostics(socket),
			"unregistered socket should not be traced");
		Cypress::CFB27::RegisterRedirectedSocketForDiagnostics(socket, "test redirect");
		Check(
			Cypress::CFB27::IsRedirectedSocketForDiagnostics(socket),
			"redirected socket should be traced");
		Cypress::CFB27::UnregisterRedirectedSocketForDiagnostics(socket);
		Check(
			!Cypress::CFB27::IsRedirectedSocketForDiagnostics(socket),
			"closed socket should stop being traced");
	}

	void TestTlsServerHelloDoneDetection()
	{
		const std::uint8_t serverHelloDone[]{0x0E, 0x00, 0x00, 0x00};
		const std::uint8_t certificate[]{0x0B, 0x00, 0x01, 0x00};
		Check(
			Cypress::CFB27::IsTlsServerHelloDone(serverHelloDone, sizeof(serverHelloDone)),
			"TLS ServerHelloDone should be recognized");
		Check(
			!Cypress::CFB27::IsTlsServerHelloDone(certificate, sizeof(certificate)),
			"other handshake messages should not be recognized as ServerHelloDone");
	}

	void TestProtoSslStateTransitionDetection()
	{
		Check(
			!Cypress::CFB27::IsProtoSslStateTransition(2, 2),
			"unchanged ProtoSSL state should not be logged as a transition");
		Check(
			Cypress::CFB27::IsProtoSslStateTransition(2, 3),
			"ProtoSSL failure state transition should be observable");
	}

	void TestProtoSslReceiveLength()
	{
		Check(
			Cypress::CFB27::BoundedProtoSslReceiveLength(-1, 512) == 0,
			"failed ProtoSSL receive should not produce a payload preview");
		Check(
			Cypress::CFB27::BoundedProtoSslReceiveLength(80, 32) == 32,
			"ProtoSSL payload preview should not exceed the caller buffer");
		Check(
			Cypress::CFB27::ProtoSslDiagnosticPreviewLimit() == 4096,
			"ProtoSSL diagnostics should retain a complete small control-plane response");
	}

}

int main()
{
	TestEnvironmentConfig();
	TestSHA256();
	TestUniquePattern();
	TestBridgeLog();
	TestRedirectorMatching();
	TestCandidateEndpointParsing();
	TestRedirectedSocketDiagnosticTracking();
	TestTlsServerHelloDoneDetection();
	TestProtoSslStateTransitionDetection();
	TestProtoSslReceiveLength();
	if (s_failures == 0)
		std::cout << "CFB27 bridge tests passed\n";
	return s_failures == 0 ? 0 : 1;
}
