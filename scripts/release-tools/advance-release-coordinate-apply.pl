#!/usr/bin/env perl
# Apply a measured coordinate move to the source tree.
#
# A move from FROM (published) to TO is a shift by one coordinate: what named
# the predecessor of FROM now names FROM, and what named FROM now names TO. The
# old predecessor pins are read from the tree itself (the lineage declaration
# and the Homebrew publisher), the new ones from the measurement JSON that
# measure-published-release.sh produced, so no value is typed by hand.
#
# The shift is never applied blindly. Each file is listed in the rules file
# with how it moves: a whole-file shift with explicit history lines kept, an
# accumulating row cloned forward, or a literal phase-list append. History is
# whatever is not explicitly moved.
use strict;
use warnings;
use JSON::PP ();

sub fail { print STDERR "advance coordinate: $_[0]\n"; exit 1 }
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

my %opt;
while (@ARGV) {
  my $key = shift @ARGV;
  $key =~ /^--(root|measurements|from-tag|from-phase|to-tag|to-phase|rules|context-out)$/ or fail("unknown option $key");
  @ARGV or fail("$key needs a value");
  $opt{$1} = shift @ARGV;
}
for my $key (qw(root measurements from-tag from-phase to-tag to-phase rules)) {
  defined $opt{$key} or fail("--$key is required");
}
my $root = $opt{root};
my ($from_tag, $from_phase, $to_tag, $to_phase) = @opt{qw(from-tag from-phase to-tag to-phase)};
$from_tag =~ /^v(\d+\.\d+\.\d+)$/ or fail('FROM_TAG is not a release tag');
my $from_version = $1;
$to_tag =~ /^v(\d+\.\d+\.\d+)$/ or fail('TO_TAG is not a release tag');
my $to_version = $1;
my ($from_n) = $from_phase =~ /^A(\d+)$/ or fail('FROM_PHASE is not a phase label');
$to_phase eq 'A' . ($from_n + 1) or fail("TO_PHASE must follow $from_phase directly");

my $m = eval { JSON::PP::decode_json(slurp($opt{measurements})) } or fail('measurement JSON is malformed');
$m->{schema_version} eq 'autopus.adk_published_release_measurement.v1' or fail('measurement schema differs');
$m->{tag} eq $from_tag && $m->{version} eq $from_version or fail("measurement is not of $from_tag");
$m->{repository} eq 'autopus-ai/autopus-adk' or fail('measurement repository differs');
my %new = (
  RELEASE_ID => $m->{release}{id}, COMMIT_SHA => $m->{commit}, TREE_SHA => $m->{tree},
  TAG_OBJECT_SHA => $m->{tag_object}, CHECKSUMS_SHA256 => $m->{checksums_sha256},
  AMD64_ARCHIVE_SHA256 => $m->{archives}{darwin_amd64}, ARM64_ARCHIVE_SHA256 => $m->{archives}{darwin_arm64},
  LINUX_AMD64_ARCHIVE_SHA256 => $m->{archives}{linux_amd64},
  LINUX_ARM64_ARCHIVE_SHA256 => $m->{archives}{linux_arm64},
  AMD64_MANIFEST_SHA256 => $m->{manifests}{darwin_amd64}, ARM64_MANIFEST_SHA256 => $m->{manifests}{darwin_arm64},
  PRIOR_TAP_COMMIT => $m->{homebrew}{tap_head}, PRIOR_CASK_BLOB => $m->{homebrew}{cask_blob},
);
for my $name (sort keys %new) {
  my $value = $new{$name} // '';
  my $shape = $name eq 'RELEASE_ID' ? qr/^[1-9][0-9]*$/
    : $name =~ /SHA256$/ ? qr/^[0-9a-f]{64}$/ : qr/^[0-9a-f]{40}$/;
  $value =~ $shape or fail("measured $name is malformed");
}

# Old values come from the tree: the predecessor's lineage declaration and the
# Homebrew publisher's prior pins.
my $coordinates_path = "$root/scripts/companion-release/verify-public-key-lineage-coordinates.sh";
my $coords = slurp($coordinates_path);
my ($prev) = $coords =~ /release_phase='\Q$from_phase\E' prior_phase='(A\d+)'/
  or fail("lineage coordinates have no $from_phase branch");
$prev eq 'A' . ($from_n - 1) or fail("$from_phase predecessor is $prev, not sequential");
my ($prev2) = $coords =~ /release_phase='\Q$prev\E' prior_phase='(A\d+)'/
  or fail("lineage coordinates have no $prev branch");
my ($prev_tag, $prev_version) = $coords =~ /\b\Q$prev\E_TAG='(v[0-9.]+)' \Q$prev\E_VERSION='([0-9.]+)'/
  or fail("lineage coordinates do not declare ${prev}_TAG");
$coords =~ /\b\Q$from_phase\E_TAG='\Q$from_tag\E' \Q$from_phase\E_VERSION='\Q$from_version\E'/
  or fail("lineage coordinates do not declare ${from_phase}_TAG as $from_tag");
$coords !~ /\b\Q$to_phase\E_TAG=/ or fail("lineage coordinates already declare ${to_phase}_TAG");
my $bridge = slurp("$root/scripts/companion-release/publish-homebrew-formula-bridge.sh");
my %old;
for my $name (keys %new) {
  my ($text, $key) = $name =~ /^PRIOR_/ ? ($bridge, $name) : ($coords, "${prev}_$name");
  ($old{$name}) = $text =~ /^readonly \Q$key\E='([0-9a-f]+|[0-9]+)'$/m or fail("cannot read $key from the tree");
}
my ($frozen) = $bridge =~ /^readonly FROZEN_FORMULA_BLOB='([0-9a-f]{40})'$/m or fail('cannot read FROZEN_FORMULA_BLOB');

# Chain checks: the measurement must extend exactly what the tree pins.
($m->{predecessor_commit} // '') eq $old{COMMIT_SHA}
  or fail("measurement did not verify ancestry from the $prev commit $old{COMMIT_SHA}; rerun with --predecessor-commit");
$m->{homebrew}{tap_head_parent} eq $old{PRIOR_TAP_COMMIT}
  or fail('measured tap head does not build on the pinned PRIOR_TAP_COMMIT');
$m->{homebrew}{formula_blob} eq $frozen or fail('measured Formula blob is not the frozen one');

my %vars = (
  PREV2_PHASE => $prev2, PREV_PHASE => $prev, FROM_PHASE => $from_phase, TO_PHASE => $to_phase,
  PREV_TAG => $prev_tag, FROM_TAG => $from_tag, TO_TAG => $to_tag,
  PREV_VERSION => $prev_version, FROM_VERSION => $from_version, TO_VERSION => $to_version,
);
# The runbook generator reads the same resolved coordinate instead of parsing
# the tree a second time.
if (defined $opt{'context-out'}) {
  spit($opt{'context-out'}, JSON::PP->new->canonical->pretty->encode({ vars => \%vars, old => \%old, new => \%new }));
}
sub expand_literal { my ($t) = @_; $t =~ s/\{([A-Z0-9_]+)\}/defined $vars{$1} ? $vars{$1} : fail("unknown placeholder $1")/ge; $t }
sub expand_regex { my ($t) = @_; $t =~ s/\{([A-Z0-9_]+)\}/defined $vars{$1} ? quotemeta($vars{$1}) : fail("unknown placeholder $1")/ge; qr/$t/ }

# One simultaneous substitution per line, so A44 -> A45 never chains into A46.
sub build_shift {
  my ($with_prev2) = @_;
  my %map = ($prev => $from_phase, $from_phase => $to_phase);
  $map{$prev2} = $prev if $with_prev2;
  my @alternatives = map { '(?<![0-9])' . quotemeta($_) . '(?![0-9])' } keys %map;
  for my $pair ([$prev_version, $from_version], [$from_version, $to_version]) {
    $map{$pair->[0]} = $pair->[1];
    push @alternatives, '(?<![0-9.])' . quotemeta($pair->[0]) . '(?![0-9])';
  }
  for my $name (keys %old) {
    $map{$old{$name}} = $new{$name};
    push @alternatives, '(?<![0-9a-f])' . quotemeta($old{$name}) . '(?![0-9a-f])';
  }
  my $regex = join '|', sort { length($b) <=> length($a) } @alternatives;
  return sub { my ($line) = @_; $line =~ s/($regex)/$map{$1}/g; $line };
}
my $shift2 = build_shift(0);
my $shift3 = build_shift(1);

my $rules_path = $opt{rules} =~ m{^/} ? $opt{rules} : "./$opt{rules}";
my $rules = do $rules_path;
ref $rules eq 'ARRAY' or fail("rules file did not load: " . ($@ || $!));
my $changed = 0;
my @writes;
for my $rule (@$rules) {
  my $path = "$root/$rule->{file}";
  my $text = slurp($path);
  my @lines = split /(?<=\n)/, $text;
  my @fixed = (0) x @lines;
  for my $clone (@{ $rule->{clone} || [] }) {
    my $anchor = expand_regex($clone->{anchor});
    my @hits = grep { $lines[$_] =~ $anchor } 0 .. $#lines;
    @hits == 1 or fail("$rule->{file}: clone anchor $clone->{anchor} matched " . scalar(@hits) . ' lines');
    my ($start, $end) = ($hits[0], $hits[0]);
    if ($clone->{up}) { my $up = expand_regex($clone->{up}); $start-- while $start > 0 && $lines[$start] !~ $up; $lines[$start] =~ $up or fail("$rule->{file}: clone start not found") }
    if ($clone->{down}) { my $down = expand_regex($clone->{down}); $end++ while $end < $#lines && $lines[$end] !~ $down; $lines[$end] =~ $down or fail("$rule->{file}: clone end not found") }
    my @copy = map { $shift3->($_) } @lines[$start .. $end];
    index($text, join('', @copy)) < 0 or fail("$rule->{file}: $to_phase clone is already present");
    splice @lines, $end + 1, 0, @copy;
    splice @fixed, $end + 1, 0, (1) x @copy;
    $fixed[$_] = 1 for $start .. $end;
  }
  for my $keep (map { expand_regex($_) } @{ $rule->{keep} || [] }) {
    my $hits = 0;
    for my $i (0 .. $#lines) { next unless $lines[$i] =~ $keep; $fixed[$i] = 1; $hits++ }
    $hits or fail("$rule->{file}: keep rule $keep matched nothing; the history it protects moved");
  }
  for my $append (@{ $rule->{append} || [] }) {
    my ($find, $replace, $count) = (expand_literal($append->[0]), expand_literal($append->[1]), $append->[2]);
    my $hits = 0;
    for my $i (0 .. $#lines) { $hits += ($lines[$i] =~ s/\Q$find\E/$replace/g) ; $fixed[$i] = 1 if $lines[$i] =~ /\Q$replace\E/ }
    $hits == $count or fail("$rule->{file}: '$find' matched $hits times, want $count");
  }
  my @only = map { expand_regex($_) } @{ $rule->{shift_only} || [] };
  if ($rule->{shift} || @only) {
    for my $i (0 .. $#lines) {
      next if $fixed[$i];
      next if @only && !grep { $lines[$i] =~ $_ } @only;
      $lines[$i] = $shift2->($lines[$i]);
    }
  }
  my $result = join '', @lines;
  if ($result eq $text) {
    fail("$rule->{file}: rule changed nothing") unless $rule->{optional};
    printf "  no pin   %s\n", $rule->{file};
    next;
  }
  push @writes, [$path, $result, $rule->{file}];
}
# Nothing is written until every rule has matched, so a refused move leaves
# the tree exactly as it was.
for my $write (@writes) {
  spit($write->[0], $write->[1]);
  $changed++;
  printf "  moved    %s\n", $write->[2];
}
printf "applied %d file(s): %s=%s -> %s=%s (predecessor %s=%s)\n",
  $changed, $from_phase, $from_tag, $to_phase, $to_tag, $prev, $prev_tag;
