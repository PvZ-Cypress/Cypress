#include <CFB27/BridgeConfig.h>

#include <Windows.h>
#include <bcrypt.h>

#include <array>
#include <charconv>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <iomanip>
#include <sstream>
#include <vector>

namespace
{
	std::string Trim(const std::string& value);

	bool TryReadEnvironment(const char* name, std::string& value)
	{
		const DWORD required = GetEnvironmentVariableA(name, nullptr, 0);
		if (required == 0)
			return false;
		std::string result(required, '\0');
		const DWORD written = GetEnvironmentVariableA(name, result.data(), required);
		if (written == 0 || written >= required)
			return false;
		result.resize(written);
		value = std::move(result);
		return true;
	}

	std::string ReadEnvironment(const char* name, const char* fallback)
	{
		std::string value;
		return TryReadEnvironment(name, value) ? value : fallback;
	}

	bool TryParsePort(const std::string& value, std::uint16_t& port)
	{
		unsigned int parsed = 0;
		const auto result = std::from_chars(value.data(), value.data() + value.size(), parsed);
		if (result.ec != std::errc() || result.ptr != value.data() + value.size() ||
			parsed == 0 || parsed > 65535)
			return false;
		port = static_cast<std::uint16_t>(parsed);
		return true;
	}

	bool TryReadPort(const char* name, std::uint16_t& port)
	{
		std::string value;
		return TryReadEnvironment(name, value) && TryParsePort(value, port);
	}

	bool TryParseBool(std::string value, bool& parsed)
	{
		value = Trim(value);
		for (char& ch : value)
			ch = static_cast<char>(std::tolower(static_cast<unsigned char>(ch)));

		if (value == "1" || value == "true" || value == "yes" || value == "on")
		{
			parsed = true;
			return true;
		}
		if (value == "0" || value == "false" || value == "no" || value == "off")
		{
			parsed = false;
			return true;
		}
		return false;
	}

	bool TryReadBool(const char* name, bool& parsed)
	{
		std::string value;
		return TryReadEnvironment(name, value) && TryParseBool(value, parsed);
	}

	std::string DefaultPrivateRoot()
	{
		const std::string appData = ReadEnvironment("APPDATA", "");
		if (appData.empty())
			return ".";
		return appData + "\\Cypress\\CFB27\\Private";
	}

	std::string DefaultRunDirectory()
	{
		return DefaultPrivateRoot() + "\\last-run";
	}

	std::string CurrentExecutableDirectory()
	{
		char path[MAX_PATH] = {};
		if (GetModuleFileNameA(nullptr, path, MAX_PATH) == 0)
			return ".";
		const auto parent = std::filesystem::path(path).parent_path();
		return parent.empty() ? std::string(".") : parent.string();
	}

	std::string Trim(const std::string& value)
	{
		std::size_t first = 0;
		while (first < value.size() && std::isspace(static_cast<unsigned char>(value[first])))
			++first;
		std::size_t last = value.size();
		while (last > first && std::isspace(static_cast<unsigned char>(value[last - 1])))
			--last;
		return value.substr(first, last - first);
	}

	void ApplyConfigValue(Cypress::CFB27::BridgeConfig& config, const std::string& key, const std::string& value)
	{
		if (key == "blazeHost")
			config.blazeHost = value;
		else if (key == "blazePort")
		{
			std::uint16_t port = 0;
			if (TryParsePort(value, port))
				config.blazePort = port;
		}
		else if (key == "profile")
			config.profile = value;
		else if (key == "runDirectory")
			config.runDirectory = value;
		else if (key == "endpointsFile")
			config.endpointsFile = value;
		else if (key == "enableBearSslBypass")
			TryParseBool(value, config.enableBearSslBypass);
		else if (key == "dumpRuntimeCodeBytes")
			TryParseBool(value, config.dumpRuntimeCodeBytes);
		else if (key == "enableRedirectorNetworkRedirect")
			TryParseBool(value, config.enableRedirectorNetworkRedirect);
		else if (key == "enableCandidateEndpointRedirects")
			TryParseBool(value, config.enableCandidateEndpointRedirects);
		else if (key == "enableProtoSslVerifyProbe")
			TryParseBool(value, config.enableProtoSslVerifyProbe);
		else if (key == "enableCertVerifyHook")
			TryParseBool(value, config.enableCertVerifyHook);
		else if (key == "certVerifyForce")
			TryParseBool(value, config.certVerifyForce);
		else if (key == "enableFailStateWatch")
			TryParseBool(value, config.enableFailStateWatch);
	}

	bool ReadConfigFile(const std::string& path, Cypress::CFB27::BridgeConfig& config)
	{
		std::ifstream stream(path);
		if (!stream.is_open())
			return false;

		std::string line;
		while (std::getline(stream, line))
		{
			const auto comment = line.find_first_of("#;");
			if (comment != std::string::npos)
				line.resize(comment);
			line = Trim(line);
			if (line.empty())
				continue;

			const auto separator = line.find('=');
			if (separator == std::string::npos)
				continue;
			const std::string key = Trim(line.substr(0, separator));
			const std::string value = Trim(line.substr(separator + 1));
			ApplyConfigValue(config, key, value);
		}

		config.configSource = path;
		return true;
	}

	void ApplyFirstConfigFile(Cypress::CFB27::BridgeConfig& config)
	{
		std::string configuredPath;
		if (TryReadEnvironment("CYPRESS_CFB27_BRIDGE_CONFIG", configuredPath))
		{
			if (ReadConfigFile(configuredPath, config))
				return;
		}

		const std::array<std::string, 2> candidates{
			DefaultPrivateRoot() + "\\cfb27-bridge.ini",
			CurrentExecutableDirectory() + "\\cfb27-bridge.ini",
		};
		for (const auto& candidate : candidates)
		{
			if (ReadConfigFile(candidate, config))
				return;
		}
	}
}

namespace Cypress::CFB27
{
	namespace
	{
		constexpr SupportedBuild SupportedBuilds[] = {
			{"CollegeFB27_Trial.exe", SupportedTrialSHA256, true},
			{"CollegeFB27.exe", SupportedNormalSHA256, false},
			{"CollegeFB27.exe", SupportedCurrentNormalSHA256, false},
			{"CollegeFB27.exe (2026-07-16)", SupportedJuly16NormalSHA256, false},
		};
	}

	BridgeConfig BridgeConfig::FromEnvironment()
	{
		BridgeConfig config;
		config.blazeHost = "127.0.0.1";
		config.blazePort = 27920;
		config.profile = "LocalPlayer";
		config.runDirectory = DefaultRunDirectory();
		config.endpointsFile = "cfb27-endpoints.json";
		config.configSource = "defaults";
		config.enableBearSslBypass = false;
		config.dumpRuntimeCodeBytes = false;
		config.enableRedirectorNetworkRedirect = true;
		config.enableCandidateEndpointRedirects = false;
		config.enableProtoSslVerifyProbe = false;
		config.enableCertVerifyHook = false;
		config.certVerifyForce = false;
		config.enableFailStateWatch = false;

		ApplyFirstConfigFile(config);

		std::string value;
		if (TryReadEnvironment("CYPRESS_CFB27_BLAZE_HOST", value))
			config.blazeHost = value;
		std::uint16_t port = 0;
		if (TryReadPort("CYPRESS_CFB27_BLAZE_PORT", port))
			config.blazePort = port;
		if (TryReadEnvironment("CYPRESS_CFB27_PROFILE", value))
			config.profile = value;
		if (TryReadEnvironment("CYPRESS_CFB27_RUN_DIR", value))
			config.runDirectory = value;
		if (TryReadEnvironment("CYPRESS_CFB27_ENDPOINTS_FILE", value))
			config.endpointsFile = value;
		bool enabled = false;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_BEARSSL_BYPASS", enabled))
			config.enableBearSslBypass = enabled;
		if (TryReadBool("CYPRESS_CFB27_DUMP_RUNTIME_CODE", enabled))
			config.dumpRuntimeCodeBytes = enabled;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_REDIRECTOR_NETWORK_REDIRECT", enabled))
			config.enableRedirectorNetworkRedirect = enabled;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_CANDIDATE_ENDPOINT_REDIRECTS", enabled))
			config.enableCandidateEndpointRedirects = enabled;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_PROTOSSL_PROBE", enabled))
			config.enableProtoSslVerifyProbe = enabled;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_CERT_HOOK", enabled))
			config.enableCertVerifyHook = enabled;
		if (TryReadBool("CYPRESS_CFB27_CERT_FORCE", enabled))
			config.certVerifyForce = enabled;
		if (TryReadBool("CYPRESS_CFB27_ENABLE_FAILWATCH", enabled))
			config.enableFailStateWatch = enabled;
		return config;
	}

	std::string SHA256FileHex(const std::string& path)
	{
		HANDLE file = CreateFileA(
			path.c_str(),
			GENERIC_READ,
			FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE,
			nullptr,
			OPEN_EXISTING,
			FILE_ATTRIBUTE_NORMAL,
			nullptr);
		if (file == INVALID_HANDLE_VALUE)
			return {};

		BCRYPT_ALG_HANDLE algorithm = nullptr;
		BCRYPT_HASH_HANDLE hash = nullptr;
		std::vector<UCHAR> hashObject;
		std::array<UCHAR, 32> digest{};
		std::string result;

		do
		{
			if (BCryptOpenAlgorithmProvider(&algorithm, BCRYPT_SHA256_ALGORITHM, nullptr, 0) < 0)
				break;

			DWORD objectSize = 0;
			DWORD copied = 0;
			if (BCryptGetProperty(
				algorithm,
				BCRYPT_OBJECT_LENGTH,
				reinterpret_cast<PUCHAR>(&objectSize),
				sizeof(objectSize),
				&copied,
				0) < 0)
				break;

			hashObject.resize(objectSize);
			if (BCryptCreateHash(
				algorithm,
				&hash,
				hashObject.data(),
				static_cast<ULONG>(hashObject.size()),
				nullptr,
				0,
				0) < 0)
				break;

			std::array<UCHAR, 64 * 1024> buffer{};
			for (;;)
			{
				DWORD bytesRead = 0;
				if (!ReadFile(file, buffer.data(), static_cast<DWORD>(buffer.size()), &bytesRead, nullptr))
					break;
				if (bytesRead == 0)
				break;
				if (BCryptHashData(hash, buffer.data(), bytesRead, 0) < 0)
				break;
			}

			if (BCryptFinishHash(hash, digest.data(), static_cast<ULONG>(digest.size()), 0) < 0)
				break;

			std::ostringstream text;
			text << std::uppercase << std::hex << std::setfill('0');
			for (const UCHAR byte : digest)
				text << std::setw(2) << static_cast<unsigned int>(byte);
			result = text.str();
		}
		while (false);

		if (hash)
			BCryptDestroyHash(hash);
		if (algorithm)
			BCryptCloseAlgorithmProvider(algorithm, 0);
		CloseHandle(file);
		return result;
	}

	const SupportedBuild* FindSupportedBuildBySHA256(const std::string& digest)
	{
		for (const auto& build : SupportedBuilds)
		{
			if (digest == build.sha256)
				return &build;
		}
		return nullptr;
	}

	const SupportedBuild* FindSupportedBuild(const std::string& executablePath)
	{
		return FindSupportedBuildBySHA256(SHA256FileHex(executablePath));
	}

	bool IsSupportedTrialBuild(const std::string& executablePath)
	{
		const SupportedBuild* build = FindSupportedBuild(executablePath);
		return build && build->trial;
	}

	bool IsProtoSslStateTransition(const std::uint32_t before, const std::uint32_t after)
	{
		return before != after;
	}

	std::size_t BoundedProtoSslReceiveLength(const std::int64_t result, const std::size_t requestedLength)
	{
		if (result <= 0)
			return 0;
		return (std::min)(static_cast<std::size_t>(result), requestedLength);
	}

	std::size_t ProtoSslDiagnosticPreviewLimit()
	{
		return 4096;
	}

}
