package companionmanifest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

var immutableA0LineagePins = map[string]string{
	"A0_COMMIT_SHA":            "7372a484eaf87a07e224476a6161f792b73d7dfb",
	"A0_RECEIPT_SHA256":        "4a588fa4991c515e9520861af5567fd2fe4c19e2c23adb8963bd37ebc46a5bbc",
	"A0_SIGNATURE_SHA256":      "7f248929d807b689acab575888b0a7600bd2ea17cce1e5fcc11f72af9c510173",
	"A0_RECORD_SHA256":         "84ee9403223aabd1f60e5e55e79a5c7d6b2c764bc594435cbf7c4e997e2ce475",
	"A0_PUBLIC_KEY_SHA256":     "c387da9e9c43dbaa2605207a00635c84937ff397a8b6ed73414d2e66b89941a4",
	"A0_CHECKSUMS_SHA256":      "17f7591ec789071e0d03c547d2a79565269de1cc13bdbc173d3703ad77947904",
	"A0_AMD64_MANIFEST_SHA256": "162dd3b21781ba59a099d41802771e2a31b3f1f80607f6dd832249803e2abdbf",
	"A0_ARM64_MANIFEST_SHA256": "8f9e28f9a0672f0e2fdb99e55027650407fd9def2d1d62ea2313b88cd83c3f61",
}

var immutableA1LineagePins = map[string]string{
	"A1_COMMIT_SHA":            "e25e8be02b55b9385f58919c30ad1ccf92179030",
	"A1_TAG_OBJECT_SHA":        "c6c72fa99234e3d8687e1c138b976fe7a62c5e00",
	"A1_CHECKSUMS_SHA256":      "b9c8ad8b5e93228277d514ec8e246290664c6d28b473c3c80ae65b8510bcda9c",
	"A1_AMD64_ARCHIVE_SHA256":  "9728aec2f36bb43b4fbb658ca8550527d371a4c570ee7fbd2aee2b6fe011e8bd",
	"A1_ARM64_ARCHIVE_SHA256":  "a57c0c180c0d2bb8ef013b9ae706752c432ff43466e13314b8b6f9279761fe4c",
	"A1_AMD64_MANIFEST_SHA256": "09b4e206fa94e4be1e2aebf6924ab8d0f349f23aaa217c33505685efb55ee163",
	"A1_ARM64_MANIFEST_SHA256": "db3a7a5381d2fa2f9e70682324b59304c5beeaaf695e91d2621f880dc7211230",
}

var immutableA2LineagePins = map[string]string{
	"A2_COMMIT_SHA":            "7b5b52822b0cda75bf6c971f5f1c2a713881008c",
	"A2_TAG_OBJECT_SHA":        "0088a9f1201e0bb11a940aa9dc4aff83aef1656c",
	"A2_CHECKSUMS_SHA256":      "5317226720dff159e73d692cc0cb447fddb134fe7c6d2046031adb377fb60092",
	"A2_AMD64_ARCHIVE_SHA256":  "babce99376a647e801ea06d99f3575c87414551cbbeb77dfeed5cfa23851b964",
	"A2_ARM64_ARCHIVE_SHA256":  "fbe9693d3517bdbaf92f230d7aa7561b728ba002749c2d06b6eef08170fed60b",
	"A2_AMD64_MANIFEST_SHA256": "82d8e22a3943dd8efc14dafd0c28ac11d415b1c1a8ff5447beb658a5cff11be4",
	"A2_ARM64_MANIFEST_SHA256": "f780452da57ec0a845bd8dae22dcd134b920c593c9ba61f496380136f243c8c0",
}

var immutableA3LineagePins = map[string]string{
	"A3_COMMIT_SHA":            "ba5509b692a43dc8a70e0bd6173acb56166ed67f",
	"A3_TAG_OBJECT_SHA":        "19fd06cec4f60218b07727e649f9671b27c1f7a7",
	"A3_CHECKSUMS_SHA256":      "1c88282d9cc215c4a766059ab9da79eecbb42126535f54b2d201e7f1309b35fe",
	"A3_AMD64_ARCHIVE_SHA256":  "064c994fd739616fabfd7b353511d633d3b73b41912f756ee8e6b655ea9366ad",
	"A3_ARM64_ARCHIVE_SHA256":  "c218a8df21ac7a7fe459e294942aa9e5b2e676d0a90a644bf486b4452f628a23",
	"A3_AMD64_MANIFEST_SHA256": "2a88c6f40e8bda35c9342fac496ed424187d25f1fa2ac0be0c714bb10c9c1490",
	"A3_ARM64_MANIFEST_SHA256": "80243b2fc0409d7743b9a7f94eaa88091ec1c53382b7215169157be314767e7e",
}

func TestReleasePublicKeyReceipt_GoReleaserA0FixtureFailsClosedOnTampering(t *testing.T) {
	tools := newExecutableLineageTools(t)
	evidence := produceGoReleaserA0FixtureEvidence(t, tools)
	t.Run("normal", func(t *testing.T) {
		fixture := newExecutableLineageFixture(t, tools, evidence)
		output, err := fixture.run(t)
		if err != nil {
			t.Fatalf("valid A0 to A1 lineage failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "A1 exact A0 key record verified") {
			t.Fatalf("valid lineage diagnostic = %q", output)
		}
	})

	cases := []struct {
		name   string
		code   string
		tamper func(*testing.T, *executableLineageFixture)
	}{
		{name: "release_not_immutable", code: "prior_evidence_unverifiable: A0 release is not immutable and final", tamper: tamperLineageReleaseImmutable},
		{name: "archive_bytes", code: "prior_archive_checksum_mismatch", tamper: tamperLineageArchiveBytes},
		{name: "checksums", code: "prior_checksums_bytes_mismatch", tamper: tamperLineageChecksums},
		{name: "embedded_manifest", code: "prior_manifest_digest_mismatch", tamper: tamperLineageManifest},
		{name: "arm64_manifest_pin", code: "prior_manifest_digest_mismatch: prior manifest differs from its A0 pin", tamper: tamperLineageARM64ManifestPin},
		{name: "receipt_key_overlap", code: "prior_key_overlap_mismatch", tamper: tamperLineageReceiptKey},
		{name: "phase", code: "prior_release_identity_mismatch", tamper: tamperLineagePhase},
		{name: "version", code: "prior_manifest_version_mismatch", tamper: tamperLineageVersion},
		{name: "signing_key", code: "prior_evidence_unverifiable", tamper: tamperLineageSigningKey},
		{name: "receipt_pin", code: "prior_receipt_bytes_mismatch: prior receipt differs from its A0 pin", tamper: tamperLineageReceiptPin},
		{name: "signature_pin", code: "prior_signature_bytes_mismatch: prior signature differs from its A0 pin", tamper: tamperLineageSignaturePin},
		{name: "public_key_pin", code: "prior_public_key_digest_mismatch", tamper: tamperLineagePublicKeyPin},
		{name: "record_pin", code: "prior_record_digest_mismatch", tamper: tamperLineageRecordPin},
		{name: "commit_pin", code: "prior_release_identity_mismatch", tamper: tamperLineageCommitPin},
		{name: "tag_commit", code: "prior_release_identity_mismatch", tamper: tamperLineageTagCommit},
		{name: "target_commit", code: "prior_release_identity_mismatch", tamper: tamperLineageTargetCommit},
		{name: "archive_entry", code: "prior_evidence_absent", tamper: tamperLineageArchiveEntry},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newExecutableLineageFixture(t, tools, evidence)
			test.tamper(t, fixture)
			output, err := fixture.run(t)
			if err == nil {
				t.Fatalf("tampered lineage passed\n%s", output)
			}
			if !strings.Contains(output, test.code) {
				t.Fatalf("tamper diagnostic = %q, want %s", output, test.code)
			}
			secret := base64.StdEncoding.EncodeToString(fixture.privateKey)
			if strings.Contains(output, secret) || strings.Contains(output, fixture.token) {
				t.Fatal("lineage diagnostic disclosed test credential material")
			}
		})
	}
}

func tamperLineageReleaseImmutable(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	var release executableLineageRelease
	if err := json.Unmarshal(readLineageFile(t, fixture.releaseJSON), &release); err != nil {
		t.Fatal(err)
	}
	release.Immutable = false
	writeLineageJSON(t, fixture.releaseJSON, release)
}

func tamperLineageArchiveBytes(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.archiveMutation = &lineageArchiveMutation{
		architecture: "amd64",
		entry:        "README.md",
		mutate: func(_ *testing.T, data []byte) ([]byte, bool) {
			return append(data, '\n'), true
		},
	}
	fixture.writeEvidence(t)
}

func tamperLineageChecksums(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.checksums = bytes.Clone(fixture.checksums)
	fixture.checksums[0] ^= 1
	fixture.writeEvidence(t)
}

func tamperLineageManifest(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.archiveMutation = replaceLineageArchiveBytes(
		t, "adk-companion-manifest.json", []byte("github-actions:"), []byte("github-actionx:"),
	)
	fixture.writeResealedEvidence(t)
}

func tamperLineageReceiptKey(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.archiveMutation = replaceLineageArchiveBytes(
		t, lineageBundleName+"/public-key-receipt.json",
		[]byte(`"key_id":"release-key"`), []byte(`"key_id":"other-key"`),
	)
	fixture.writeResealedEvidence(t)
}

func tamperLineagePhase(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.releaseTag = "v0.50.70"
	fixture.writeEvidence(t)
}

func tamperLineageVersion(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.archiveMutation = replaceLineageArchiveBytes(
		t, "adk-companion-manifest.json", []byte(`"version":"0.50.69"`), []byte(`"version":"9.9.9"`),
	)
	fixture.writeResealedEvidence(t)
}

func replaceLineageArchiveBytes(
	t *testing.T,
	entryName string,
	old, replacement []byte,
) *lineageArchiveMutation {
	t.Helper()
	return &lineageArchiveMutation{
		entry: entryName,
		mutate: func(t *testing.T, entry []byte) ([]byte, bool) {
			if bytes.Count(entry, old) != 1 {
				t.Fatalf("tamper target %s is not exact", entryName)
			}
			return bytes.Replace(entry, old, replacement, 1), true
		},
	}
}

func tamperLineageSigningKey(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.writeSigningKey(t, bytes.Repeat([]byte{0x42}, 32))
}

func tamperLineageReceiptPin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.receipt = differentHex(fixture.pins.receipt, 64)
}

func tamperLineageSignaturePin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.signature = differentHex(fixture.pins.signature, 64)
}

func tamperLineageARM64ManifestPin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.arm64Manifest = differentHex(fixture.pins.arm64Manifest, 64)
}

func tamperLineagePublicKeyPin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.publicKey = differentHex(fixture.pins.publicKey, 64)
}

func tamperLineageRecordPin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.record = differentHex(fixture.pins.record, 64)
}

func tamperLineageCommitPin(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.pins.commit = differentHex(fixture.pins.commit, 40)
}

func tamperLineageTagCommit(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.tagCommit = differentHex(fixture.tagCommit, 40)
	fixture.writeEvidence(t)
}

func tamperLineageTargetCommit(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.targetCommit = differentHex(fixture.targetCommit, 40)
	fixture.writeEvidence(t)
}

func tamperLineageArchiveEntry(t *testing.T, fixture *executableLineageFixture) {
	t.Helper()
	fixture.archiveMutation = &lineageArchiveMutation{
		entry: lineageBundleName + "/public-key-receipt.sig",
		mutate: func(*testing.T, []byte) ([]byte, bool) {
			return nil, false
		},
	}
	fixture.writeResealedEvidence(t)
}

func differentHex(current string, length int) string {
	candidate := strings.Repeat("a", length)
	if current == candidate {
		return strings.Repeat("b", length)
	}
	return candidate
}

func TestReleasePublicKeyReceipt_ProductionPinsHaveNoRuntimeTestOverride(t *testing.T) {
	pinsSource := string(releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-pins.sh"))
	runtimeSource := string(releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage.sh")) + pinsSource +
		string(releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-shipped-pins.sh"))
	for _, pins := range []map[string]string{
		immutableA0LineagePins, immutableA1LineagePins, immutableA2LineagePins,
		immutableA3LineagePins, immutableA4LineagePins, immutableA5LineagePins,
		immutableA6LineagePins, immutableA7LineagePins, immutableA8LineagePins,
		immutableA9LineagePins, immutableA10LineagePins, immutableA11LineagePins,
		immutableA12LineagePins, immutableA13LineagePins, immutableA14LineagePins,
		immutableA15LineagePins, immutableA16LineagePins, immutableA17LineagePins,
		immutableA18LineagePins, immutableA19LineagePins, immutableA20LineagePins,
		immutableA21LineagePins,
	} {
		for name, value := range pins {
			declaration := "readonly " + name + "='" + value + "'"
			if strings.Count(pinsSource, declaration) != 1 {
				t.Fatalf("production immutable pin declaration drifted: %s", declaration)
			}
		}
	}
	for _, bypass := range []string{
		"TEST_PIN", "PIN_FILE", "PIN_OVERRIDE", "GO_WANT_LINEAGE",
		"COMPANION_A0_", "COMPANION_A1_", "COMPANION_A2_", "COMPANION_A3_", "COMPANION_A4_", "COMPANION_A5_", "COMPANION_A6_", "COMPANION_A7_", "COMPANION_A8_", "COMPANION_A9_", "COMPANION_A10_", "COMPANION_A11_", "COMPANION_A12_", "COMPANION_A13_", "COMPANION_A14_", "COMPANION_A15_", "COMPANION_A16_", "COMPANION_A17_", "COMPANION_A18_", "COMPANION_A19_", "COMPANION_A20_", "COMPANION_A21_",
	} {
		if strings.Contains(runtimeSource, bypass) {
			t.Fatalf("production lineage exposes test pin bypass %q", bypass)
		}
	}
	for _, required := range []string{
		`[[ -f "$pins_helper" && ! -L "$pins_helper" ]]`,
		`source "$pins_helper"`,
	} {
		if !strings.Contains(runtimeSource, required) {
			t.Fatalf("production lineage pin helper gate missing %q", required)
		}
	}
}
