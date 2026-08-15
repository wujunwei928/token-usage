package codex

import (
	"encoding/binary"
	"math/bits"
	"sort"
	"strings"
	"sync"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// Kind selects the report granularity.
type Kind int

// Report kinds.
const (
	KindDaily Kind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// Report kinds as their rows_key names.
func (k Kind) String() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "session"
	}
}

// UsageBucket is the per-tier usage split used for cost calculation.
type UsageBucket struct {
	InputTokens                  uint64
	CachedInputTokens            uint64
	OutputTokens                 uint64
	LongContextInputTokens       uint64
	LongContextCachedInputTokens uint64
	LongContextOutputTokens      uint64
}

// ModelUsage aggregates one model's events inside a group, including the
// long-context and recorded-speed splits that per-request pricing needs.
type ModelUsage struct {
	InputTokens                  uint64
	CachedInputTokens            uint64
	OutputTokens                 uint64
	ReasoningOutputTokens        uint64
	TotalTokens                  uint64
	LongContextInputTokens       uint64
	LongContextCachedInputTokens uint64
	LongContextOutputTokens      uint64
	RecordedStandardUsage        UsageBucket
	RecordedFastUsage            UsageBucket
	IsFallback                   bool
}

// Group is one period's aggregate (date, week, month, or session id).
type Group struct {
	InputTokens           uint64
	CachedInputTokens     uint64
	OutputTokens          uint64
	ReasoningOutputTokens uint64
	TotalTokens           uint64
	Models                map[string]*ModelUsage
	LastActivity          *string
}

func newGroup() *Group {
	return &Group{Models: map[string]*ModelUsage{}}
}

// GroupEntry pairs a period label with its group for sorted iteration.
type GroupEntry struct {
	Period string
	Group  *Group
}

// Groups is an insertion-ordered period map with sorted iteration, mirroring
// the reference's BTreeMap<String, CodexGroup>.
type Groups struct {
	byPeriod map[string]*Group
}

func newGroups() *Groups {
	return &Groups{byPeriod: map[string]*Group{}}
}

// Entry returns the group for period, creating it when missing.
func (g *Groups) Entry(period string) *Group {
	if group, ok := g.byPeriod[period]; ok {
		return group
	}
	group := newGroup()
	g.byPeriod[period] = group
	return group
}

// Get returns the group for period, or nil.
func (g *Groups) Get(period string) *Group {
	return g.byPeriod[period]
}

// Len reports the number of periods.
func (g *Groups) Len() int {
	return len(g.byPeriod)
}

// Sorted returns the periods and groups in key order.
func (g *Groups) Sorted() []GroupEntry {
	periods := make([]string, 0, len(g.byPeriod))
	for period := range g.byPeriod {
		periods = append(periods, period)
	}
	sort.Strings(periods)
	entries := make([]GroupEntry, 0, len(periods))
	for _, period := range periods {
		entries = append(entries, GroupEntry{Period: period, Group: g.byPeriod[period]})
	}
	return entries
}

// SortedModels returns the model names of a group in key order.
func (group *Group) SortedModels() []string {
	models := make([]string, 0, len(group.Models))
	for model := range group.Models {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

// ---------------------------------------------------------------------------
// Dedupe
// ---------------------------------------------------------------------------

// fxHasher replicates rustc-hash's FxHasher so (hash, len) key pairs behave
// exactly like the reference's dedupe keys.
type fxHasher struct{ hash uint64 }

const fxSeed64 = 0x51_7c_c1_b7_27_22_0a_95

func (h *fxHasher) addToHash(i uint64) {
	h.hash = (bits.RotateLeft64(h.hash, 5) ^ i) * fxSeed64
}

func (h *fxHasher) write(p []byte) {
	for len(p) >= 8 {
		h.addToHash(binary.LittleEndian.Uint64(p))
		p = p[8:]
	}
	if len(p) >= 4 {
		h.addToHash(uint64(binary.LittleEndian.Uint32(p)))
		p = p[4:]
	}
	if len(p) >= 2 {
		h.addToHash(uint64(binary.LittleEndian.Uint16(p)))
		p = p[2:]
	}
	if len(p) >= 1 {
		h.addToHash(uint64(p[0]))
	}
}

// hashText mirrors Rust's Hash for str: bytes plus the 0xff terminator.
func hashText(value string) uint64 {
	h := &fxHasher{}
	h.write([]byte(value))
	h.addToHash(0xff)
	return h.hash
}

// eventKey is the dedupe key: matching usage records collapse across session
// files (session identity only participates for session reports).
type eventKey struct {
	sessionHash           uint64
	sessionLen            int
	timestamp             int64
	modelHash             uint64
	modelLen              int
	inputTokens           uint64
	cachedInputTokens     uint64
	outputTokens          uint64
	reasoningOutputTokens uint64
	totalTokens           uint64
}

type dedupeRecord struct {
	serviceTier *ServiceTier
	model       string
	sessionID   *string
}

type dedupeMap struct {
	mu   sync.Mutex
	seen map[eventKey]dedupeRecord
}

func newDedupeMap() *dedupeMap {
	return &dedupeMap{seen: map[eventKey]dedupeRecord{}}
}

func (d *dedupeMap) insert(key eventKey, event *TokenUsageEvent, model string, kind Kind) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if record, ok := d.seen[key]; ok {
		record.serviceTier = mergeServiceTiers(record.serviceTier, event.ServiceTier)
		d.seen[key] = record
		return false
	}
	record := dedupeRecord{serviceTier: event.ServiceTier, model: model}
	if kind == KindSession {
		sessionID := event.SessionID
		record.sessionID = &sessionID
	}
	d.seen[key] = record
	return true
}

func codexEventKey(event *TokenUsageEvent, timestamp int64, model string, kind Kind) eventKey {
	key := eventKey{
		timestamp:             timestamp,
		modelHash:             hashText(model),
		modelLen:              len(model),
		inputTokens:           event.InputTokens,
		cachedInputTokens:     event.CachedInputTokens,
		outputTokens:          event.OutputTokens,
		reasoningOutputTokens: event.ReasoningOutputTokens,
		totalTokens:           event.TotalTokens,
	}
	if kind == KindSession {
		key.sessionHash = hashText(event.SessionID)
		key.sessionLen = len(event.SessionID)
	}
	return key
}

// ---------------------------------------------------------------------------
// Group loading
// ---------------------------------------------------------------------------

// aggregateRun holds the read-only inputs every file of one aggregation pass
// shares.
type aggregateRun struct {
	sessionsDir string
	files       []string
	shared      *core.SharedArgs
	kind        Kind
	replayPlan  *ReplayPlan
}

// timestampAbort carries the invalid-timestamp abort out of the visit
// callback, which cannot return an error itself.
type timestampAbort struct{ err error }

// LoadGroups aggregates usage into period groups from every configured Codex
// home.
func LoadGroups(shared *core.SharedArgs, kind Kind) (groups *Groups, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if abort, ok := recovered.(timestampAbort); ok {
				groups, err = nil, abort.err
				return
			}
			panic(recovered)
		}
	}()
	sources, err := usageSources()
	if err != nil {
		return nil, err
	}
	if len(sources) == 1 && !core.WantsJSON(shared) {
		return loadGroupsFromDirectory(sources[0].Dir, shared, kind)
	}
	return loadGroupsFromSources(sources, shared, kind)
}

func loadGroupsFromSources(sources []UsageSource, shared *core.SharedArgs, kind Kind) (*Groups, error) {
	fileGroups := CollectDedupedUsageFiles(sources)
	plan := NewReplayPlan(fileGroups, shared.SingleThread)
	groups := newGroups()
	seen := newDedupeMap()
	for _, group := range fileGroups {
		run := &aggregateRun{
			sessionsDir: group.Dir,
			files:       group.Files,
			shared:      shared,
			kind:        kind,
			replayPlan:  plan,
		}
		merged := aggregateFilesWithDedupe(run, seen)
		mergeGroups(groups, merged)
	}
	applyRecordedUsage(groups, seen.all(), shared, kind)
	return groups, nil
}

func loadGroupsFromDirectory(sessionsDir string, shared *core.SharedArgs, kind Kind) (*Groups, error) {
	files := CollectUsageFiles(sessionsDir)
	plan := NewReplayPlan([]UsageFileGroup{{Dir: sessionsDir, Files: files}}, shared.SingleThread)
	run := &aggregateRun{
		sessionsDir: sessionsDir,
		files:       files,
		shared:      shared,
		kind:        kind,
		replayPlan:  plan,
	}
	seen := newDedupeMap()
	groups := aggregateFilesWithDedupe(run, seen)
	applyRecordedUsage(groups, seen.all(), shared, kind)
	return groups, nil
}

func aggregateFilesWithDedupe(run *aggregateRun, seen *dedupeMap) *Groups {
	perFile := common.ReadFilesParallel(run.files, run.shared.SingleThread, func(path string) *Groups {
		groups := newGroups()
		VisitSessionFile(run.sessionsDir, path, run.replayPlan.ReplayPrefix(path), func(event TokenUsageEvent) {
			addEventToGroups(&event, run.kind, run.shared, seen, groups)
		})
		return groups
	})
	groups := newGroups()
	for _, fileGroups := range perFile {
		mergeGroups(groups, fileGroups)
	}
	return groups
}

func addEventToGroups(event *TokenUsageEvent, kind Kind, shared *core.SharedArgs, seen *dedupeMap, groups *Groups) {
	model := ""
	if event.Model != nil && *event.Model != "" {
		model = core.ResolveModelName(*event.Model)
	} else {
		return
	}
	timestamp, ok := core.ParseTSTimestamp(event.Timestamp)
	if !ok {
		// The reference propagates this out of the visit callback and aborts
		// the whole load; a sentinel panic unwinds the same way.
		panic(timestampAbort{err: invalidTimestampError(event.Timestamp)})
	}
	key := codexEventKey(event, timestamp, model, kind)
	if !seen.insert(key, event, model, kind) {
		return
	}
	addDedupedEventToGroups(event, model, timestamp, kind, shared, groups)
}

func addDedupedEventToGroups(event *TokenUsageEvent, model string, timestamp int64, kind Kind, shared *core.SharedArgs, groups *Groups) {
	period, ok := codexPeriodFor(timestamp, event.SessionID, kind, shared)
	if !ok {
		return
	}
	group := groups.Entry(period)
	accumulateEventIntoGroup(group, event, model, false)
}

func codexPeriodFor(timestamp int64, sessionID string, kind Kind, shared *core.SharedArgs) (string, bool) {
	date := core.FormatDateTZ(timestamp, core.ParseTZ(shared.Timezone))
	if shared.Since != nil || shared.Until != nil {
		dateKey := date
		dateKey = strings.ReplaceAll(dateKey, "-", "")
		if shared.Since != nil && dateKey < *shared.Since {
			return "", false
		}
		if shared.Until != nil && dateKey > *shared.Until {
			return "", false
		}
	}
	switch kind {
	case KindDaily:
		return date, true
	case KindWeekly:
		if week := core.WeekStart(date, core.Monday); week != "" {
			return week, true
		}
		return date, true
	case KindMonthly:
		return date[:7], true
	default:
		if sessionID == "" {
			return "", false
		}
		return sessionID, true
	}
}

func accumulateEventIntoGroup(group *Group, event *TokenUsageEvent, model string, recordServiceTier bool) {
	group.InputTokens += event.InputTokens
	group.CachedInputTokens += event.CachedInputTokens
	group.OutputTokens += event.OutputTokens
	group.ReasoningOutputTokens += event.ReasoningOutputTokens
	group.TotalTokens += event.TotalTokens
	if group.LastActivity == nil || event.Timestamp > *group.LastActivity {
		timestamp := event.Timestamp
		group.LastActivity = &timestamp
	}
	usage, ok := group.Models[model]
	if !ok {
		usage = &ModelUsage{}
		group.Models[model] = usage
	}
	usage.InputTokens += event.InputTokens
	usage.CachedInputTokens += event.CachedInputTokens
	usage.OutputTokens += event.OutputTokens
	usage.ReasoningOutputTokens += event.ReasoningOutputTokens
	usage.TotalTokens += event.TotalTokens
	// Each event is one request, so its input size decides the pricing tier
	// here; summed totals cannot recover per-request context sizes.
	isLongContext := event.InputTokens > core.LongContextSplitThreshold(model)
	if isLongContext {
		usage.LongContextInputTokens += event.InputTokens
		usage.LongContextCachedInputTokens += event.CachedInputTokens
		usage.LongContextOutputTokens += event.OutputTokens
	}
	if recordServiceTier && event.ServiceTier != nil {
		var recorded *UsageBucket
		switch *event.ServiceTier {
		case TierStandard:
			recorded = &usage.RecordedStandardUsage
		case TierFast:
			recorded = &usage.RecordedFastUsage
		}
		if recorded != nil {
			accumulateBucket(recorded, event, isLongContext)
		}
	}
	usage.IsFallback = usage.IsFallback || event.IsFallbackModel
}

func accumulateBucket(bucket *UsageBucket, event *TokenUsageEvent, isLongContext bool) {
	bucket.InputTokens += event.InputTokens
	bucket.CachedInputTokens += event.CachedInputTokens
	bucket.OutputTokens += event.OutputTokens
	if isLongContext {
		bucket.LongContextInputTokens += event.InputTokens
		bucket.LongContextCachedInputTokens += event.CachedInputTokens
		bucket.LongContextOutputTokens += event.OutputTokens
	}
}

func mergeBucket(target *UsageBucket, source UsageBucket) {
	target.InputTokens += source.InputTokens
	target.CachedInputTokens += source.CachedInputTokens
	target.OutputTokens += source.OutputTokens
	target.LongContextInputTokens += source.LongContextInputTokens
	target.LongContextCachedInputTokens += source.LongContextCachedInputTokens
	target.LongContextOutputTokens += source.LongContextOutputTokens
}

func (d *dedupeMap) all() map[eventKey]dedupeRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[eventKey]dedupeRecord, len(d.seen))
	for key, record := range d.seen {
		out[key] = record
	}
	return out
}

// applyRecordedUsage replays the dedupe records' service tiers into the
// groups' recorded-speed buckets (the parallel load path accumulates them
// after the fact so shards stay lock-free).
func applyRecordedUsage(groups *Groups, records map[eventKey]dedupeRecord, shared *core.SharedArgs, kind Kind) {
	for key, record := range records {
		if record.serviceTier == nil {
			continue
		}
		sessionID := ""
		if record.sessionID != nil {
			sessionID = *record.sessionID
		}
		period, ok := codexPeriodFor(key.timestamp, sessionID, kind, shared)
		if !ok {
			continue
		}
		group := groups.Get(period)
		if group == nil {
			continue
		}
		usage := group.Models[record.model]
		if usage == nil {
			continue
		}
		isLongContext := key.inputTokens > core.LongContextSplitThreshold(record.model)
		bucket := UsageBucket{
			InputTokens:       key.inputTokens,
			CachedInputTokens: key.cachedInputTokens,
			OutputTokens:      key.outputTokens,
		}
		if isLongContext {
			bucket.LongContextInputTokens = key.inputTokens
			bucket.LongContextCachedInputTokens = key.cachedInputTokens
			bucket.LongContextOutputTokens = key.outputTokens
		}
		switch *record.serviceTier {
		case TierStandard:
			mergeBucket(&usage.RecordedStandardUsage, bucket)
		case TierFast:
			mergeBucket(&usage.RecordedFastUsage, bucket)
		}
	}
}

func mergeGroups(target *Groups, source *Groups) {
	for _, entry := range source.Sorted() {
		group := target.Entry(entry.Period)
		group.InputTokens += entry.Group.InputTokens
		group.CachedInputTokens += entry.Group.CachedInputTokens
		group.OutputTokens += entry.Group.OutputTokens
		group.ReasoningOutputTokens += entry.Group.ReasoningOutputTokens
		group.TotalTokens += entry.Group.TotalTokens
		if entry.Group.LastActivity != nil &&
			(group.LastActivity == nil || *entry.Group.LastActivity > *group.LastActivity) {
			timestamp := *entry.Group.LastActivity
			group.LastActivity = &timestamp
		}
		for model, usage := range entry.Group.Models {
			targetUsage, ok := group.Models[model]
			if !ok {
				targetUsage = &ModelUsage{}
				group.Models[model] = targetUsage
			}
			targetUsage.InputTokens += usage.InputTokens
			targetUsage.CachedInputTokens += usage.CachedInputTokens
			targetUsage.OutputTokens += usage.OutputTokens
			targetUsage.ReasoningOutputTokens += usage.ReasoningOutputTokens
			targetUsage.TotalTokens += usage.TotalTokens
			targetUsage.LongContextInputTokens += usage.LongContextInputTokens
			targetUsage.LongContextCachedInputTokens += usage.LongContextCachedInputTokens
			targetUsage.LongContextOutputTokens += usage.LongContextOutputTokens
			mergeBucket(&targetUsage.RecordedStandardUsage, usage.RecordedStandardUsage)
			mergeBucket(&targetUsage.RecordedFastUsage, usage.RecordedFastUsage)
			targetUsage.IsFallback = targetUsage.IsFallback || usage.IsFallback
		}
	}
}

// ---------------------------------------------------------------------------
// Event-list aggregation (used by the unified report with date filters)
// ---------------------------------------------------------------------------

// AggregateEvents groups already-loaded events by period, recording service
// tiers inline.
func AggregateEvents(events []TokenUsageEvent, kind Kind, timezone *string) (*Groups, error) {
	groups := newGroups()
	for i := range events {
		event := &events[i]
		model := ""
		if event.Model != nil && *event.Model != "" {
			model = *event.Model
		} else {
			continue
		}
		timestamp, ok := core.ParseTSTimestamp(event.Timestamp)
		if !ok {
			return nil, invalidTimestampError(event.Timestamp)
		}
		date := core.FormatDateTZ(timestamp, core.ParseTZ(timezone))
		var period string
		switch kind {
		case KindDaily:
			period = date
		case KindWeekly:
			if week := core.WeekStart(date, core.Monday); week != "" {
				period = week
			} else {
				period = date
			}
		case KindMonthly:
			period = date[:7]
		default:
			period = event.SessionID
		}
		group := groups.Entry(period)
		accumulateEventIntoGroup(group, event, core.ResolveModelName(model), true)
	}
	return groups, nil
}

// FilterEventsByDate keeps only the events inside the inclusive since/until
// window.
func FilterEventsByDate(events *[]TokenUsageEvent, shared *core.SharedArgs) error {
	if shared.Since == nil && shared.Until == nil {
		return nil
	}
	kept := make([]TokenUsageEvent, 0, len(*events))
	for i := range *events {
		event := &(*events)[i]
		timestamp, ok := core.ParseTSTimestamp(event.Timestamp)
		if !ok {
			return invalidTimestampError(event.Timestamp)
		}
		date := core.FormatDateTZ(timestamp, core.ParseTZ(shared.Timezone))
		date = strings.ReplaceAll(date, "-", "")
		if (shared.Since == nil || date >= *shared.Since) &&
			(shared.Until == nil || date <= *shared.Until) {
			kept = append(kept, *event)
		}
	}
	*events = kept
	return nil
}

func invalidTimestampError(timestamp string) error {
	return &core.CLIError{Message: "Invalid Codex timestamp: " + timestamp}
}
