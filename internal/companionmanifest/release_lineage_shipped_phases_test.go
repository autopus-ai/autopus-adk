package companionmanifest

// A22 이후의 좌표 행이다. 여기가 릴리즈마다 자라는 쪽이라 frozenReleasePhases와 분리했다.
// 발행된 행은 덮지 않고, 새 phase를 그 아래에 더한다.
var shippedReleasePhases = []releasePhase{
	{
		phase: "A22", tag: "v0.50.109", version: "0.50.109",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "b86fab067599f457261287552c5a9dd86460d7f4",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"ADK_KEY_ROTATION_VERIFIED",
			"release-tag-signing-2026-q3-r2.pub",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsLinuxArchives: true, pinsReleaseID: true,
	},
	{
		phase: "A23", tag: "v0.50.111", version: "0.50.111",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "67f3def5d4a0a11aadd9e103389de6cc1cafc34e",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// v0.50.112 is absent on purpose. It was armed as A24, tagged, and pushed,
		// but CI failed at the tagged commit so the release job never ran and the
		// coordinate was burned. A burned coordinate never enters this table, which
		// is why v0.50.110 and v0.50.75..76 are absent too. A24 is v0.50.113.
		phase: "A24", tag: "v0.50.113", version: "0.50.113",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "954f60a77acb59fd4106537020693fdcadb3d640",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A25 shipped on omp/17.2.7. Its first attempt carried omp/18.1.5 and
		// failed closed in the cohort at call 6 of 42 before any tag existed, so
		// the coordinate survived and the pin went back to the contract version.
		phase: "A25", tag: "v0.50.114", version: "0.50.114",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "bc2147a875b49e9fca75db4307455f83512837d6",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A26 follows the published A25 on the same omp/17.2.7 oracle; the
		// predecessor pins are measured from immutable release 382345734.
		phase: "A26", tag: "v0.50.115", version: "0.50.115",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "a6d199fb5a7b27721026916fcd75dffb58a4e228",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A27 follows the published A26 on the same omp/17.2.7 oracle; the
		// predecessor pins are measured from immutable release 383249963.
		phase: "A27", tag: "v0.50.116", version: "0.50.116",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "77ae668bf7e9eb8d0dae177d1c9b7e41a5d51ef6",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A28 follows the published A27 on the same omp/17.2.7 oracle; the
		// predecessor pins are measured from immutable release 383500138.
		phase: "A28", tag: "v0.50.117", version: "0.50.117",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "fbe502c05f84d5eeb81b089b2344c47329ab4543",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A29 follows the published A28 and is the first coordinate armed on the
		// omp/18.1.13 pin, so its cohort measurement is what the release proves;
		// the predecessor pins are measured from immutable release 383826825.
		phase: "A29", tag: "v0.50.118", version: "0.50.118",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "620e29a44d004cb199d5f1c22ae92878f9b6930e",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A30 follows A29, the first release published after the transfer to
		// autopus-ai, on the retained omp/17.2.7 pin; the predecessor pins are
		// measured from immutable release 392314996.
		phase: "A30", tag: "v0.50.119", version: "0.50.119",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "4480c8d2f6c00c205ee838cd4bd20933bfff3597",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A31 follows A30 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 398256974.
		phase: "A31", tag: "v0.50.120", version: "0.50.120",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "279bc98635639a91e08285c5ffc649d8f4c7df26",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A32 follows A31 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 398906327.
		phase: "A32", tag: "v0.50.121", version: "0.50.121",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "b69a8450d6a1c916dfd6e86b254907b4b0c76b78",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A33 follows A32 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 402213224.
		phase: "A33", tag: "v0.50.122", version: "0.50.122",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "8299c8df85bc0e24bfb4d4fbbd71889dceb96986",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A34 follows A33 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 402813712.
		phase: "A34", tag: "v0.50.123", version: "0.50.123",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "c42337f4dcf9592d065130150af18dcc5a16c5b8",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
	{
		// A35 follows A34 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 402971620.
		phase: "A35", tag: "v0.50.124", version: "0.50.124",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "c447badc28e393b19984d2eeea159a81d609acd9",
		extraSourceGates: []string{
			"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED",
			"release-tag-signing-2026-q3-r2.pub",
			"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
			`verify-tag "refs/tags/$GITHUB_REF_NAME"`,
		},
		pinsRepository: true, pinsEvidenceSource: true, pinsTagObject: true,
		pinsReleaseID: true, callerTreeSHA: true, callerReleaseID: true,
		bridgePredecessor: true,
	},
}
