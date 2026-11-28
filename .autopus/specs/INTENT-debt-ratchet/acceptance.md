<!-- intent_source: request. Written by `auto qa intent` from request.md alone, without the code.
     Record answers with `auto qa intent answer`; scenario generation reads them from questions.json. -->

### AC-1: `auto debt` lists debt items by location and kind
Given a repository with the harness installed, containing a product source file with an untracked `TODO` comment, a product source file close to the file-size limit, and the same code block repeated in two product source files
When the user runs `auto debt`
Then the command exits with code 0 and its output lists each item with its file path, line number, and kind (size-limit, TODO-type comment, or duplicate block)

### AC-2: All four markers are counted, but markers with an issue number or link are not
Given a product source file with untracked comments starting with `TODO`, `FIXME`, `HACK`, and `XXX`, plus a `TODO(#123)` comment and a `FIXME https://example.com/issues/9` comment
When the user runs `auto debt`
Then the output lists the four untracked comments as TODO-type items and lists neither the comment with the issue number nor the comment with the link

### AC-3: Comment detection works in any language
Given product source files in several languages, each with an untracked TODO comment in that language's syntax (for example `# TODO` in Python, `// TODO` in Go, `-- TODO` in SQL)
When the user runs `auto debt`
Then each of those comments appears in the output as a TODO-type item

### AC-4: A file near the size limit is reported and a small file is not
Given `architecture.max_file_lines` is 300, product source file A has 290 lines, and product source file B has 100 lines
When the user runs `auto debt`
Then A is listed as a size-limit item and B is not listed as a size-limit item

### AC-5: A repeated code block is reported with every place it appears
Given the same multi-line code block appears in two product source files
When the user runs `auto debt`
Then the output lists a duplicate-block item that names both file:line locations

### AC-6: Tests, docs, generated files, and harness directories are not counted
Given TODO comments, repeated blocks, and near-limit files that exist only in test files, documentation files, generated files, and the harness directory
When the user runs `auto debt`
Then none of those appear in the output

### AC-7: Creating the baseline for the first time
Given a repository with debt items and no baseline file
When the user runs the `auto debt` baseline-write command
Then a baseline file appears in the repository, ready to commit, that lists every item `auto debt` currently reports, by file, line, and kind

### AC-8: Rewriting the baseline after cleanup makes it smaller
Given a committed baseline with 5 items, and 2 of them have since been removed from the working tree
When the user runs the baseline-write command
Then the baseline file lists only the 3 items that remain

### AC-9: The baseline rewrite will not quietly accept new debt
Given a committed baseline, and the working tree has an item that is not in the baseline
When the user runs the baseline-write command without an explicit override
Then the command exits with a non-zero code, names the new item (file:line, kind), and leaves the baseline file unchanged

### AC-10: The Stop hook blocks once on new debt and names each new item
Given a committed baseline, and an agent adds an untracked `TODO` comment at `src/x.go:42` in the working tree
When `auto verify stop` runs
Then it blocks, and its output names `src/x.go:42` and the TODO-type kind

### AC-11: The same new items only warn the next time
Given `auto verify stop` already blocked on a set of new debt items, and the working tree has not gained any further new items
When `auto verify stop` runs again
Then it does not block, and its output still shows those items as a warning

### AC-12: An item added after the first block triggers a new block
Given `auto verify stop` already blocked on item P, and the agent then adds another new item Q
When `auto verify stop` runs
Then it blocks, and its output names Q as new

### AC-13: Items are compared one by one, not by total
Given a baseline that contains TODO item A, and an agent removes A and adds a different TODO item B, so the total count stays the same
When `auto verify stop` runs
Then it blocks and names B as a new item

### AC-14: A file nearing the size limit only warns
Given a product source file that is not in the baseline grows close to `architecture.max_file_lines`, and there are no other new items
When `auto verify stop` runs
Then it does not block, and its output shows a warning naming that file as a size-limit item

### AC-15: Debt already in the baseline does not block
Given every debt item in the working tree is already listed in the baseline
When `auto verify stop` runs
Then it does not block because of debt

### AC-16: Moving an existing item to a new line does not make it new
Given TODO item A is in the baseline at `src/y.py:10`, and an agent inserts 5 lines above it so it is now at line 15 with the same text
When `auto verify stop` runs
Then it does not block on A and does not report A as new

### AC-17: The debt ratchet can be turned off
Given a new untracked TODO that is not in the baseline, and the debt ratchet is turned off in the harness configuration
When `auto verify stop` runs
Then it neither blocks nor warns about debt, and its other checks (build, tests, secrets, file-size limit) still run as before

### AC-18: Without a baseline file, the hook does not block
Given a repository with debt items and no baseline file
When `auto verify stop` runs
Then it does not block because of debt, and its output tells the user to create a baseline with `auto debt`

### AC-19: The weekly cleanup command gives ordered instructions, largest item first
Given a committed baseline with several items of different sizes
When a person or a scheduled job runs the weekly cleanup command of `auto debt`
Then the command exits with code 0 and prints cleanup instructions that list items from largest to smallest, say what to change for each, and end with these steps: rewrite the baseline smaller, then open a PR for a person to merge

### AC-20: The weekly cleanup command with nothing to clean up
Given the baseline and the working tree contain no debt items
When the weekly cleanup command runs
Then it exits with code 0 and says there is nothing to clean up

### AC-21: The Stop hook does not get noticeably slower
Given a repository of typical size with a committed baseline
When `auto verify stop` runs with the debt ratchet on, and again with it off
Then the run with the ratchet on takes at most 1 second longer than the run with it off
