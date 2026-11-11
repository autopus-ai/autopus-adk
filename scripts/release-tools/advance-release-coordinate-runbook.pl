#!/usr/bin/env perl
# Write the runbooks of a coordinate move: the new TO runbook from
# docs/runbooks/release-coordinate-template.md, and the Published record at the
# top of the FROM runbook. Every value comes from the measurement JSON, the
# resolved coordinate that advance-release-coordinate-apply.pl wrote, or the
# record flags the caller measured through gh; nothing is typed by hand.
use strict;
use warnings;
use JSON::PP ();

sub fail { print STDERR "advance coordinate runbook: $_[0]\n"; exit 1 }
sub slurp {
  my ($path) = @_;
  -f $path && !-l $path or fail("missing or unsafe $path");
  open(my $fh, '<', $path) or fail("cannot read $path");
  local $/; my $text = <$fh>; close $fh; return $text;
}
sub spit {
  my ($path, $text) = @_;
  open(my $fh, '>', $path) or fail("cannot write $path");
  print {$fh} $text; close $fh or fail("cannot write $path");
}

my %opt = (note => '');
my @keys = qw(root measurements context base date release-run release-conclusion canary-run
  canary-conclusion ruleset-id ruleset-state tag-policy-id note);
while (@ARGV) {
  my $key = shift @ARGV;
  $key =~ /^--([a-z-]+)$/ && grep { $_ eq $1 } @keys or fail("unknown option $key");
  @ARGV or fail("$key needs a value");
  $opt{$1} = shift @ARGV;
}
for my $key (grep { $_ ne 'note' } @keys) { defined $opt{$key} && length $opt{$key} or fail("--$key is required") }
$opt{date} =~ /^\d{4}-\d{2}-\d{2}$/ or fail('--date is not YYYY-MM-DD');
$opt{base} =~ /^[0-9a-f]{8,40}$/ or fail('--base is not a commit');
for my $key (qw(release-run canary-run ruleset-id tag-policy-id)) {
  $opt{$key} =~ /^[1-9][0-9]*$/ or fail("--$key is not a numeric id");
}

my $m = JSON::PP::decode_json(slurp($opt{measurements}));
my $c = JSON::PP::decode_json(slurp($opt{context}));
my %v = %{ $c->{vars} };
my ($major, $minor, $patch) = $v{TO_VERSION} =~ /^(\d+)\.(\d+)\.(\d+)$/ or fail('TO_VERSION is malformed');
my ($to_n) = $v{TO_PHASE} =~ /^A(\d+)$/;
my %t = (
  %v, DATE => $opt{date}, BASE => substr($opt{base}, 0, 8),
  NEXT_TAG => "v$major.$minor." . ($patch + 1), NEXT_PHASE => 'A' . ($to_n + 1),
  RELEASE_ID => $m->{release}{id}, PUBLISHED_AT => $m->{release}{published_at},
  ASSET_COUNT => $m->{release}{asset_count}, LATEST => ($m->{release}{latest} ? 'yes' : 'no'),
  TAG_OBJECT => $m->{tag_object}, COMMIT => $m->{commit}, COMMIT8 => substr($m->{commit}, 0, 8),
  COMMIT12 => substr($m->{commit}, 0, 12), TREE => $m->{tree}, CHECKSUMS => $m->{checksums_sha256},
  DARWIN_AMD64 => $m->{archives}{darwin_amd64}, DARWIN_ARM64 => $m->{archives}{darwin_arm64},
  LINUX_AMD64 => $m->{archives}{linux_amd64}, LINUX_ARM64 => $m->{archives}{linux_arm64},
  MANIFEST_AMD64 => $m->{manifests}{darwin_amd64}, MANIFEST_ARM64 => $m->{manifests}{darwin_arm64},
  BINARY_AMD64_8 => substr($m->{binaries}{darwin_amd64}, 0, 8),
  BINARY_ARM64_8 => substr($m->{binaries}{darwin_arm64}, 0, 8),
  EVIDENCE_TAG => $m->{evidence}{tag}, EVIDENCE_OBJECT => $m->{evidence}{tag_object},
  EVIDENCE_COMMIT => $m->{evidence}{commit},
  TAP_HEAD => $m->{homebrew}{tap_head}, TAP_HEAD8 => substr($m->{homebrew}{tap_head}, 0, 8),
  TAP_PARENT8 => substr($m->{homebrew}{tap_head_parent}, 0, 8),
  CASK_BLOB => $m->{homebrew}{cask_blob}, CASK_BLOB8 => substr($m->{homebrew}{cask_blob}, 0, 8),
  FORMULA_BLOB => $m->{homebrew}{formula_blob},
  PREV_COMMIT => $c->{old}{COMMIT_SHA}, PREV_COMMIT8 => substr($c->{old}{COMMIT_SHA}, 0, 8),
);
sub render {
  my ($text) = @_;
  $text =~ s/\{\{([A-Z0-9_]+)\}\}/defined $t{$1} ? $t{$1} : fail("template names unknown {{$1}}")/ge;
  return $text;
}

my $dir = "$opt{root}/docs/runbooks";
my $next_path = "$dir/release-$v{TO_TAG}.md";
-e $next_path and fail("$next_path already exists");
my $previous_path = "$dir/release-$v{FROM_TAG}.md";
my $previous = slurp($previous_path);
$previous =~ /^# Release \Q$v{FROM_TAG}\E \(\Q$v{FROM_PHASE}\E\)\n\n/ or fail("$previous_path does not open with its title");
$previous !~ /^\*\*Published /m or fail("$previous_path already has a Published record");

my $sealed = $opt{'ruleset-state'} eq 'sealed' ? 'is sealed (no bypass actor)'
  : "is **$opt{'ruleset-state'}**, not sealed";
my ($pub_date) = $m->{release}{published_at} =~ /^(\d{4}-\d{2}-\d{2})T/ or fail("published_at is malformed");
my $note = length $opt{note} ? "\n$opt{note}\n" : '';
my $record = render(<<"RECORD");
**Published $pub_date.** Immutable release `{{RELEASE_ID}}` ({{ASSET_COUNT}} assets, Latest: {{LATEST}}),
published {{PUBLISHED_AT}}, tag object
`{{TAG_OBJECT}}`, source commit
`{{COMMIT}}`, tree
`{{TREE}}`.

Release run `$opt{'release-run'}` concluded `$opt{'release-conclusion'}`. The Homebrew tap head is
`{{TAP_HEAD8}}` ("Publish signed Cask for {{FROM_TAG}}", parent `{{TAP_PARENT8}}`), and its Cask
carries the four `checksums.txt` archive digests. Upgrade Canary
`$opt{'canary-run'}` ({{PREV_VERSION}} -> {{FROM_VERSION}}) concluded `$opt{'canary-conclusion'}`. Ruleset
`$opt{'ruleset-id'}` $sealed, tag policy `$opt{'tag-policy-id'}`.
$note
The next coordinate is {{TO_PHASE}} = `{{TO_TAG}}`; see
[release-{{TO_TAG}}.md](release-{{TO_TAG}}.md).

RECORD
$previous =~ s/^(# Release [^\n]+\n\n)/$1$record/;
my $runbook = render(slurp("$dir/release-coordinate-template.md"));
spit($next_path, $runbook);
spit($previous_path, $previous);
print "  wrote    docs/runbooks/release-$v{TO_TAG}.md\n";
print "  recorded docs/runbooks/release-$v{FROM_TAG}.md (Published)\n";
