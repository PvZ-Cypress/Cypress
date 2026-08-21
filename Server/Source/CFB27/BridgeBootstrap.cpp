#include <CFB27/BridgeBootstrap.h>

#include <CFB27/BridgeConfig.h>
#include <CFB27/BridgeLog.h>
#include <CFB27/MemoryDiscovery.h>
#include <CFB27/RedirectorHook.h>
#include <MinHook.h>

#include <Windows.h>

#include <string>

namespace
{
	Cypress::CFB27::BridgeLog s_log;

	DWORD WINAPI BridgeWorker(LPVOID)
	{
		try
		{
			const auto config = Cypress::CFB27::BridgeConfig::FromEnvironment();
			if (!s_log.Open(config.runDirectory))
			{
				char currentDirectory[MAX_PATH] = {};
				if (GetCurrentDirectoryA(MAX_PATH, currentDirectory) != 0)
					s_log.Open(currentDirectory);
			}
			s_log.Write("CFB27 bridge bootstrap started");
			s_log.Write("bridge log path=" + s_log.Path());
			s_log.Write(
				"configuration host=" + config.blazeHost +
				" port=" + std::to_string(config.blazePort) +
				" profile=" + config.profile +
				" source=" + config.configSource +
				" endpoints=" + config.endpointsFile +
				" enableBearSslBypass=" + (config.enableBearSslBypass ? "true" : "false") +
				" dumpRuntimeCodeBytes=" + (config.dumpRuntimeCodeBytes ? "true" : "false") +
				" enableCandidateEndpointRedirects=" + (config.enableCandidateEndpointRedirects ? "true" : "false"));

			char executablePath[MAX_PATH] = {};
			if (GetModuleFileNameA(nullptr, executablePath, MAX_PATH) == 0)
			{
				s_log.Write("failed to resolve the game executable path");
				return 1;
			}

			const std::string digest = Cypress::CFB27::SHA256FileHex(executablePath);
			s_log.Write("game SHA-256=" + digest);
			const Cypress::CFB27::SupportedBuild* build = Cypress::CFB27::FindSupportedBuildBySHA256(digest);
			if (!build)
			{
				s_log.Write("unsupported game build; no hooks installed");
				return 2;
			}
			s_log.Write(std::string("supported game build=") + build->name + (build->trial ? " trial" : " normal"));
			s_log.Write(std::string("game executable path=") + executablePath);

			if (MH_Initialize() != MH_OK)
			{
				s_log.Write("MinHook initialization failed");
				return 3;
			}

			if (!Cypress::CFB27::InstallRedirectorHooks(config, s_log))
			{
				s_log.Write("redirector hook installation failed");
				return 4;
			}

			Cypress::CFB27::LogImageDiscoverySummary(s_log);

			if (config.enableRedirectorNetworkRedirect)
			{
				if (!Cypress::CFB27::PatchRedirectorHostnameTable(config, s_log))
					s_log.Write("redirector hostname table patch was not available; API hooks remain active");

				if (!Cypress::CFB27::PatchRedirectorServiceNameTable(s_log))
					s_log.Write("redirector service-name table patch was not available; secure service name may remain selected");
			}
			else
			{
				s_log.Write("redirector table patches disabled; observation mode preserves the production endpoint");
			}

			if (config.enableBearSslBypass)
			{
				if (!Cypress::CFB27::InstallBearSslCertificateBypass(s_log))
					s_log.Write("BearSSL certificate validation bypass was not installed; local TLS may fail");
			}
			else
			{
				s_log.Write("BearSSL certificate validation bypass disabled by configuration");
			}

			if (config.dumpRuntimeCodeBytes)
				Cypress::CFB27::LogProtoSslRuntimeCodeBytes(s_log);
			else
				s_log.Write("ProtoSSL runtime code byte dump disabled by configuration");

			if (config.enableProtoSslVerifyProbe)
			{
				if (!Cypress::CFB27::InstallProtoSslVerifyProbe(s_log))
					s_log.Write("ProtoSSL verify probe was not installed");
			}
			else
			{
				s_log.Write("ProtoSSL verify probe disabled by configuration");
			}

			if (config.enableCertVerifyHook)
			{
				if (!Cypress::CFB27::InstallCertVerifyHook(s_log, config.certVerifyForce))
					s_log.Write("cert-verify hook was not installed");
			}
			else
			{
				s_log.Write("cert-verify hook disabled by configuration");
			}

			if (config.enableFailStateWatch)
			{
				if (!Cypress::CFB27::InstallFailStateWatch(s_log))
					s_log.Write("fail-state watch was not installed");
			}
			else
			{
				s_log.Write("fail-state watch disabled by configuration");
			}

			Cypress::CFB27::LogRuntimeRedirectorReferences(s_log);
			s_log.Write("CFB27 bridge bootstrap completed");
			return 0;
		}
		catch (...)
		{
			s_log.Write("unhandled bridge bootstrap exception");
			return 5;
		}
	}
}

namespace Cypress::CFB27
{
	void StartBridge(HMODULE)
	{
		HANDLE thread = CreateThread(nullptr, 0, BridgeWorker, nullptr, 0, nullptr);
		if (thread)
			CloseHandle(thread);
	}
}
