<!-- intent_source: request. Written by `auto qa intent` from request.md alone, without the code.
     Record answers with `auto qa intent answer`; scenario generation reads them from questions.json. -->

### AC-1: Approving a lesson produces structured instructions for the work agent
Given a lesson candidate that another provider has tried and failed to disprove
When a person approves the lesson
Then the harness outputs instructions for the work agent that include the order for picking the strongest layer (remove via API > lint/type > test > hook > text), a slot for a patch that reproduces the mistake, and a slot for the check command

### AC-2: Before proof, the approved lesson stays in the text rules file as a temporary rule
Given a lesson that has just been approved and has no proven check yet
When the user opens .claude/rules/harness-lessons.md
Then the file contains that lesson's text rule
And the lesson's state shows it as not yet proven (temporary text rule)

### AC-3: A check that fails on the patched tree and passes on the current tree is accepted as proof
Given the work agent has submitted a reproduction patch, a check command and a layer for an approved lesson
When the harness runs the check command itself on a tree with the reproduction patch applied, and again on the current tree
Then the check exits with a non-zero code on the patched tree and with exit code 0 on the current tree
And the harness reports the lesson as proven

### AC-4: A proven lesson leaves the text rules file and the state records the check
Given a lesson the harness has proven as described in AC-3
When the user opens .claude/rules/harness-lessons.md and the lesson's state
Then the lesson's text rule no longer appears in harness-lessons.md
And the lesson's state shows the check command and the chosen layer

### AC-5: A check that does not catch the reproduced mistake is rejected
Given a submitted check command that exits with code 0 on the tree with the reproduction patch applied
When the harness runs its proof
Then the harness reports that the proof failed because the check does not catch the reproduced mistake
And the lesson's text rule stays in harness-lessons.md and the lesson stays unproven

### AC-6: A check that fails on the current tree is rejected
Given a submitted check command that exits with a non-zero code on the current tree
When the harness runs its proof
Then the harness reports that the proof failed because the check does not pass on the current tree
And the lesson's text rule stays in harness-lessons.md and the lesson stays unproven

### AC-7: A reproduction patch that cannot be applied fails the proof
Given a submitted reproduction patch that does not apply cleanly to the current tree
When the harness runs its proof
Then the harness reports that the proof failed because the reproduction patch could not be applied
And the lesson's text rule stays in harness-lessons.md and the lesson stays unproven

### AC-8: The agent's own report of success is not accepted without a harness run
Given the work agent claims a check exists and passes, but the harness has not run the check on both trees
When the user looks at the lesson's state
Then the lesson is not marked proven
And no check command or layer is recorded as proven for it
And the lesson's text rule stays in harness-lessons.md

### AC-9: Proof does not change the user's current working tree
Given a working tree with a known set of files and uncommitted changes
When the harness runs a proof that applies the reproduction patch
Then after the proof, the working tree's file contents and status match what they were before

### AC-10: A lesson that cannot be turned into a check stays as a text rule, with a reason
Given the work agent reports that an approved lesson cannot be turned into a check and gives a reason
When the harness records the result
Then the lesson's text rule stays in harness-lessons.md
And the lesson's state shows the layer as text along with the stated reason

### AC-11: Giving up on a check without a reason is rejected
Given the work agent reports that an approved lesson cannot be turned into a check but gives no reason
When the harness records the result
Then the harness rejects the report and says a reason is required
And the lesson stays a temporary, unproven text rule
