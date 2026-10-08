package intake

// ProtectedLearningIDs returns the ids of the learning entries `auto learn
// prune` keeps regardless of age (REQ-HC-07): the representative and every
// learning ref of each open candidate and each promoted link record, read
// whether or not a manifest exists, and the provenance ref of every incident
// task below the manifest's active paths. A retired incident tombstone still
// names its incident, so its ref is kept too. Rejection records protect
// nothing and are not read. A project with neither an intake area nor a
// manifest protects nothing.
//
// The scan only reads, through the same os.Root helpers on every platform.
// Records decode strictly, as their wire contracts require; a task decodes
// leniently, provenance alone, so SPEC-HARNEVAL-001 semantic checks never block
// prune. Any scanned file that is unreadable, malformed, or a symlink, and any
// symlinked intake or task directory, fails the whole scan with
// eval_links_unreadable, so the caller changes nothing.
func ProtectedLearningIDs(root string) (map[string]bool, error) {
	ids, err := scanProtected(root)
	if err != nil {
		return nil, &RunError{Reason: ReasonEvalLinksUnreadable, Err: err}
	}
	return ids, nil
}

func scanProtected(root string) (map[string]bool, error) {
	a, err := openArea(root)
	if err != nil {
		return nil, err
	}
	// The scan writes nothing, so a close error loses nothing.
	defer func() { _ = a.close() }()
	if err := a.checkLayout(); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	keep := func(refs ...string) {
		for _, ref := range refs {
			if ref != "" {
				ids[ref] = true
			}
		}
	}
	scans := []struct {
		dir   string
		valid func(string) bool
		refs  func([]byte) ([]string, error)
	}{
		{IntakeDir, ValidCandidateID, candidateRefs},
		{PromotedDir, ValidTaskID, linkRefs},
	}
	for _, scan := range scans {
		err := scanRecords(a, scan.dir, scan.valid, func(_ string, data []byte) error {
			refs, err := scan.refs(data)
			keep(refs...)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	err = scanActiveTasks(a, func(task taskRef) {
		if task.Provenance.Kind == "incident" {
			keep(task.Provenance.Ref)
		}
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// candidateRefs strictly decodes an open candidate and returns the learning
// ids its group holds.
func candidateRefs(data []byte) ([]string, error) {
	var record Candidate
	if err := decodeStrict(data, &record); err != nil {
		return nil, err
	}
	if _, err := recordKey(record.SchemaVersion, CandidateSchemaV1, record.FingerprintVersion, record.Fingerprint); err != nil {
		return nil, err
	}
	return append([]string{record.Representative}, record.LearningRefs...), nil
}

// linkRefs strictly decodes a promoted link record and returns the learning
// ids of the group it moved out of the candidate.
func linkRefs(data []byte) ([]string, error) {
	var record Link
	if err := decodeStrict(data, &record); err != nil {
		return nil, err
	}
	if _, err := recordKey(record.SchemaVersion, LinkSchemaV1, record.FingerprintVersion, record.Fingerprint); err != nil {
		return nil, err
	}
	return append([]string{record.Representative}, record.LearningRefs...), nil
}
