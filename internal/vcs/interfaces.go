package vcs

import "github.com/OpticDiff/code-reviewer/pkg/vcs"

type DiffFetcher = vcs.DiffFetcher
type NotePoster = vcs.NotePoster
type Approver = vcs.Approver
type DescriptionUpdater = vcs.DescriptionUpdater
type VCSProvider = vcs.VCSProvider

type DismissedFinding = vcs.DismissedFinding
type ThreadReader = vcs.ThreadReader

var FingerprintMarker = vcs.FingerprintMarker
var ParseFingerprint = vcs.ParseFingerprint
var AnchorHash = vcs.AnchorHash
var FindingSpan = vcs.FindingSpan
var ComputeFingerprint = vcs.ComputeFingerprint

type FingerprintInfo = vcs.FingerprintInfo

const MaxAnchorLines = vcs.MaxAnchorLines
