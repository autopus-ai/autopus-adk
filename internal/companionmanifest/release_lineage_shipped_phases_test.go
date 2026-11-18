package companionmanifest

// The coordinate rows from A40 on. This is the side that grows with every
// release, so it is kept apart from frozenReleasePhases; A22..A30 moved
// unchanged to release_lineage_settled_phases_test.go and A31..A39 to
// release_lineage_published_phases_test.go under the 300-line limit.
// Published rows are never rewritten; a new phase is appended below.
var shippedReleasePhases = []releasePhase{
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
		phase: "A41", tag: "v0.51.0", version: "0.51.0",
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
	{
		// A42 follows A41 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 439021686.
		phase: "A42", tag: "v0.51.1", version: "0.51.1",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "ce109c5ae958f86b624bc40e91aaf7f6b1bf0f6d",
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
		// A43 follows A42 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 439292420.
		phase: "A43", tag: "v0.51.2", version: "0.51.2",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "48c4a16538ef4ab829c50b7852f8217477300f53",
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
		// A44 follows A43 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 439814195.
		phase: "A44", tag: "v0.51.3", version: "0.51.3",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "276d4bd16a9aec143688ab19cfd6affd01f28b00",
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
		// A45 follows A44 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 440878772.
		phase: "A45", tag: "v0.51.4", version: "0.51.4",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "4a0fb20ca37ea54efe611484d697e1e79ba0b8f1",
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
		// A46 follows A45 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 441368606.
		phase: "A46", tag: "v0.51.5", version: "0.51.5",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "4312105591a74f79ed3f62a7023423d49a2944f0",
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
		// A47 follows A46 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 441700303.
		phase: "A47", tag: "v0.51.6", version: "0.51.6",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "8ab8b182c425ab8a68112293e8ab272ff3765dcb",
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
		// A48 follows A47 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 443543192.
		phase: "A48", tag: "v0.51.7", version: "0.51.7",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "ea183465fa5955c683cca89f9b5e2bd5e2c8ed09",
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
		// A49 follows A48 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 444197313.
		phase: "A49", tag: "v0.51.8", version: "0.51.8",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "61306bee7b8d8202a398780de34551e90360fd06",
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
		// A50 follows A49 under the same autopus-ai owner; the predecessor pins
		// are measured from immutable release 449766415.
		phase: "A50", tag: "v0.51.9", version: "0.51.9",
		acceptedField: "source-tree",
		rejects:       "unsignedTag",
		ancestorSHA:   "8994d90fce86befdf7b97ce53a0b25519ff26cab",
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
