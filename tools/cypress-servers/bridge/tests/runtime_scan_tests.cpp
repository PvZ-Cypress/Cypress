#include "../runtime_scan.h"

#include <array>
#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <span>

namespace
{
	using Cypress::CFB27::RuntimeScan::FindUniquePattern;
	using Cypress::CFB27::RuntimeScan::npos;

	void Check(const bool condition, const char* message)
	{
		if (!condition)
		{
			std::cerr << "FAIL: " << message << '\n';
			std::exit(1);
		}
	}

	void TestUniqueMatch()
	{
		constexpr std::array<std::uint8_t, 7> bytes{0x10, 0x20, 0xAA, 0xBB, 0xCC, 0x30, 0x40};
		constexpr std::array<std::uint8_t, 3> pattern{0xAA, 0xBB, 0xCC};
		const auto result = FindUniquePattern(bytes, pattern);
		Check(result.count == 1, "unique pattern should have one match");
		Check(result.offset == 2, "unique pattern should be found at hand-derived offset 2");
	}

	void TestNoMatch()
	{
		constexpr std::array<std::uint8_t, 4> bytes{0x10, 0x20, 0x30, 0x40};
		constexpr std::array<std::uint8_t, 2> pattern{0xAA, 0xBB};
		const auto result = FindUniquePattern(bytes, pattern);
		Check(result.count == 0, "absent pattern should have no matches");
		Check(result.offset == npos, "absent pattern should not expose an offset");
	}

	void TestMultipleMatchesAreAmbiguous()
	{
		constexpr std::array<std::uint8_t, 7> bytes{0xAA, 0xBB, 0x10, 0xAA, 0xBB, 0x20, 0x30};
		constexpr std::array<std::uint8_t, 2> pattern{0xAA, 0xBB};
		const auto result = FindUniquePattern(bytes, pattern);
		Check(result.count == 2, "duplicate pattern should report two matches");
		Check(result.offset == npos, "duplicate pattern must not select an offset");
	}

	void TestPatternLongerThanRegion()
	{
		constexpr std::array<std::uint8_t, 2> bytes{0xAA, 0xBB};
		constexpr std::array<std::uint8_t, 3> pattern{0xAA, 0xBB, 0xCC};
		const auto result = FindUniquePattern(bytes, pattern);
		Check(result.count == 0, "oversized pattern should have no matches");
		Check(result.offset == npos, "oversized pattern should not expose an offset");
	}

	void TestFinalLegalOffset()
	{
		constexpr std::array<std::uint8_t, 5> bytes{0x10, 0x20, 0x30, 0xAA, 0xBB};
		constexpr std::array<std::uint8_t, 2> pattern{0xAA, 0xBB};
		const auto result = FindUniquePattern(bytes, pattern);
		Check(result.count == 1, "pattern at final legal offset should match");
		Check(result.offset == 3, "final legal offset should be three");
	}

	void TestVerifiedPatchMetadata()
	{
		using namespace Cypress::CFB27::RuntimeScan;
		Check(kVerifiedSignature.size() == 36, "verified runtime signature should contain 36 bytes");
		Check(kPatchOffsetInSignature == 28, "trust-result patch should begin at hand-derived offset 28");
		Check(kReplacementBytes.size() == 3, "trust-result replacement should contain three bytes");
		Check(kVerifiedSignature[kPatchOffsetInSignature] == 0x44, "verified signature should retain the original destination register move");
		Check(kReplacementBytes[0] == 0x45 && kReplacementBytes[1] == 0x33 && kReplacementBytes[2] == 0xFF,
			"replacement should encode xor r15d, r15d");
	}

	void TestPrivateLaunchLeaseWindow()
	{
		using Cypress::CFB27::RuntimeScan::PrivateLaunchLeaseIsActive;
		Check(PrivateLaunchLeaseIsActive(1000, 1001), "future private launch lease should be active");
		Check(PrivateLaunchLeaseIsActive(1000, 1000), "lease should remain active through its expiry second");
		Check(!PrivateLaunchLeaseIsActive(1000, 999), "expired private launch lease should be inactive");
		Check(!PrivateLaunchLeaseIsActive(1000, 0), "missing private launch lease should be inactive");
	}
}

int main()
{
	TestUniqueMatch();
	TestNoMatch();
	TestMultipleMatchesAreAmbiguous();
	TestPatternLongerThanRegion();
	TestFinalLegalOffset();
	TestVerifiedPatchMetadata();
	TestPrivateLaunchLeaseWindow();
	std::cout << "runtime_scan_tests: PASS\n";
	return 0;
}
