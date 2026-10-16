package vcs

import "github.com/OpticDiff/code-reviewer/pkg/vcs"

type DiffFetcher = vcs.DiffFetcher
type NotePoster = vcs.NotePoster
type Approver = vcs.Approver
type SuggestionApplier = vcs.SuggestionApplier
type DescriptionUpdater = vcs.DescriptionUpdater
type VCSProvider = vcs.VCSProvider

type DismissedFinding = vcs.DismissedFinding
type ThreadReader = vcs.ThreadReader
type OpenFinding = vcs.OpenFinding
type OpenThreadReader = vcs.OpenThreadReader

var FingerprintMarker = vcs.FingerprintMarker
var ParseFingerprint = vcs.ParseFingerprint
var ParseFindingHeader = vcs.ParseFindingHeader
var AnchorHash = vcs.AnchorHash
var FindingSpan = vcs.FindingSpan
var ComputeFingerprint = vcs.ComputeFingerprint
var FingerprintFor = vcs.FingerprintFor

type FingerprintInfo = vcs.FingerprintInfo
type FindingHeader = vcs.FindingHeader

const MaxAnchorLines = vcs.MaxAnchorLines
