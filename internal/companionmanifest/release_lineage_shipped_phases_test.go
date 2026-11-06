package companionmanifest

// The coordinate rows from A31 on. This is the side that grows with every
// release, so it is kept apart from frozenReleasePhases, and A22..A30 moved
// unchanged to release_lineage_settled_phases_test.go under the 300-line limit.
// Published rows are never rewritten; a new phase is appended below.
var shippedReleasePhases = []releasePhase{
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
		// v0.50.124 is absent on purpose. It was armed as A35, tagged, and
		// pushed, but the protected release job failed inside GoReleaser on both
		// attempts, so no asset was published and the coordinate was burned. A
		// burned coordinate never enters this table, like v0.50.112 before it.
		// A35 is v0.50.125.
		phase: "A35", tag: "v0.50.125", version: "0.50.125",
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
	{
		// A36 follows A35 under the same autopus-ai owner on the retained
		// omp/17.2.7 pin; the predecessor pins are measured from immutable
		// release 410986343.
		// v0.50.126 is absent on purpose. It was armed as A36, tagged, and
		// pushed, but the protected release job was approved only after its
		// one-day same-run evidence artifact and its K3 attestation had both
		// expired, so no asset was published and the coordinate was burned. A
		// burned coordinate never enters this table, like v0.50.124 before it.
		// A36 is v0.50.127.
		phase: "A36", tag: "v0.50.127", version: "0.50.127",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "dd6d6ea2435d759144ec093b9b464a831f47703d",
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
		// A37 follows A36 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 421737046.
		phase: "A37", tag: "v0.50.128", version: "0.50.128",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "81186d0c73bb6eb633e52a7cb6a2af23d0f54d12",
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
		// A38 follows A37 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 427217669.
		// v0.50.129 is absent on purpose. It was armed as A38, tagged, and
		// pushed, but its final signed companion execution smoke failed on the
		// macOS 15.8.2 hosted image, so no asset was published and the
		// coordinate was burned. A burned coordinate never enters this table,
		// like v0.50.124 and v0.50.126 before it. v0.50.130 is absent too: it
		// was tagged and its release run passed tag CI except macos-runtime,
		// where the post-detach stall of the ptrace gate hung a child on macOS
		// 15.8.2. Fixing that needed new source, so it was burned before any
		// approval. A38 is v0.50.131.
		phase: "A38", tag: "v0.50.131", version: "0.50.131",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "842f8bf286aa4c26554cc0d29f3c79c412069a0b",
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
		// A39 follows A38 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 429669574.
		phase: "A39", tag: "v0.50.132", version: "0.50.132",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "b27d93ab0fababe08152ca18807232075a8d1ac6",
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
		// A40 follows A39 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 432079301.
		phase: "A40", tag: "v0.50.133", version: "0.50.133",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "2b49d429f15be98aecb719e3abefc19a3612a346",
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
		// A41 follows A40 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 432232109.
		phase: "A41", tag: "v0.50.134", version: "0.50.134",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "f78feef6824a58aa534f355f2b49763b9b256bbc",
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
