#pragma once

#include <array>
#include <cstddef>
#include <cstdint>
#include <span>

namespace Cypress::CFB27::RuntimeScan
{
	inline constexpr std::size_t npos = static_cast<std::size_t>(-1);
	inline constexpr std::array<std::uint8_t, 36> kVerifiedSignature{
		0x49, 0x8B, 0xCE,
		0x41, 0xFF, 0xC5,
		0xE8, 0xAE, 0x59, 0x00, 0x00,
		0x48, 0x8D, 0x97, 0xD0, 0x47, 0x00, 0x00,
		0x41, 0xB8, 0x40, 0x0B, 0x00, 0x00,
		0x48, 0x8D, 0x4D, 0xA0,
		0x44, 0x8B, 0xF8,
		0xE8, 0x75, 0xC6, 0xFC, 0xFF,
	};
	inline constexpr std::array<std::uint8_t, 3> kReplacementBytes{0x45, 0x33, 0xFF};
	inline constexpr std::size_t kPatchOffsetInSignature = 28;

	inline constexpr bool PrivateLaunchLeaseIsActive(
		const std::uint64_t nowUtc,
		const std::uint64_t expiresUtc)
	{
		return expiresUtc != 0 && nowUtc <= expiresUtc;
	}

	struct PatternResult
	{
		std::size_t offset = npos;
		std::size_t count = 0;
	};

	inline PatternResult FindUniquePattern(
		const std::span<const std::uint8_t> bytes,
		const std::span<const std::uint8_t> pattern)
	{
		PatternResult result;
		if (pattern.empty() || pattern.size() > bytes.size())
			return result;

		for (std::size_t offset = 0; offset <= bytes.size() - pattern.size(); ++offset)
		{
			bool matches = true;
			for (std::size_t index = 0; index < pattern.size(); ++index)
			{
				if (bytes[offset + index] != pattern[index])
				{
					matches = false;
					break;
				}
			}
			if (!matches)
				continue;

			++result.count;
			result.offset = result.count == 1 ? offset : npos;
		}
		return result;
	}
}
