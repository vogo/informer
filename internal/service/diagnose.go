/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
)

// Bounds of one diagnosis run.
const (
	// DiagnoseTimeoutSeconds is the floor of a diagnosis run's time budget.
	//
	// A diagnosis is not a fetch: the agent reads the page in windows, tries a
	// candidate, reads the result and tries again. The configured fetch timeout
	// - five minutes by default - cuts that loop off halfway, so a diagnosis
	// never runs with less than this even when the fetch budget is smaller.
	DiagnoseTimeoutSeconds = 900

	// verifySampleArticles is how many verified articles a report quotes back,
	// so a person can see the configuration produced real titles before it is
	// saved. Composing a new source verifies exactly the same way, and reads the
	// same number of samples.
	verifySampleArticles = 10
)

// DiagnoseVerification is informer's own check of a proposed repair.
type DiagnoseVerification = ParseVerification

// ParseVerification is informer's own check of a proposed configuration.
//
// It exists because the agent's word is not enough: it reports what it believes
// it verified, and this is what actually happened when informer parsed the
// source with the proposal applied, in this process, just now. Repairing a
// source and composing one share it, so the bar a proposal has to clear cannot
// drift between the two.
type ParseVerification struct {
	// Ran says a verification parse was attempted at all. It is false when the
	// diagnosis proposed no change to verify.
	Ran bool `json:"ran"`

	// ArticleCount is how many articles the proposed configuration parsed out.
	ArticleCount int `json:"article_count"`

	// Samples are the first parsed articles, so the titles can be eyeballed:
	// a regex that matches the navigation bar also "works".
	Samples []*feed.Article `json:"samples"`

	// Error is why the verification parse failed, empty when it succeeded.
	Error string `json:"error"`

	// Note says why no verification ran, when none did. It exists because "not
	// verified" and "verified and failed" have to look different to the person
	// deciding whether to save: an agent candidate cannot be tried from inside
	// an agent run, and that is a fact about informer, not a fault of the
	// configuration.
	Note string `json:"note"`
}

// DiagnoseReport is the outcome of one diagnosis run.
type DiagnoseReport struct {
	// SourceID is the subscription that was diagnosed.
	SourceID int64 `json:"source_id"`

	// Fixed is informer's own verdict: a proposal exists and informer parsed
	// articles with it. Only this decides whether a fix is offered.
	Fixed bool `json:"fixed"`

	// AgentClaimedFixed is what the agent said about its own work. It is kept
	// next to Fixed so a disagreement between the two is visible rather than
	// silently resolved in the agent's favor.
	AgentClaimedFixed bool `json:"agent_claimed_fixed"`

	// Diagnosis is the explanation of what went wrong.
	Diagnosis string `json:"diagnosis"`

	// Advice is what the user can do when nothing could be repaired.
	Advice string `json:"advice"`

	// Changes are the edits worth applying: the proposal minus the fields it
	// restated without changing. It is nil when there is nothing to apply.
	Changes *diagnose.Changes `json:"changes"`

	// Diff renders Changes against the stored source, for a person to read.
	Diff []diagnose.FieldChange `json:"diff"`

	// Verification is what informer's own re-parse produced.
	Verification *DiagnoseVerification `json:"verification"`
}

// DiagnoseSource asks the configured agent why a source stopped parsing and what
// would repair it, and returns a verified proposal.
//
// Nothing is written. The source, its health state and its error message are all
// left exactly as they were: this call reads, runs an agent and parses. Applying
// the proposal is a separate, explicit ApplySourceFix, because a repair that
// turns out wrong would otherwise have overwritten the only copy of what the
// configuration used to be.
//
// sink, when not nil, receives the run's progress as it happens - the retry, the
// whole prompt, every tool call the agent makes and the verification - which for
// a run measured in minutes is the difference between a diagnosis and a spinner.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) DiagnoseSource(ctx context.Context, sourceID int64, sink runlog.Sink) (*DiagnoseReport, error) {
	source, err := s.GetSource(sourceID)
	if err != nil {
		return nil, err
	}

	s.ApplyHTTPProxy()

	// resolving the agent binary probes a login shell under its own short
	// deadline, which is not this run's budget to hand down.
	session := s.diagnoseSession(source, sink) //nolint:contextcheck //own deadline.

	dir, err := os.MkdirTemp("", "informer-diagnose-")
	if err != nil {
		return nil, fmt.Errorf("create diagnose dir: %w", err)
	}

	// the session snapshot carries whatever the source's curl line carries; it
	// exists for the length of one run and is removed with it.
	defer func() {
		removeErr := os.RemoveAll(dir)
		if removeErr != nil {
			runlog.Warnf(sink, "清理诊断临时目录失败：%v", removeErr)
		}
	}()

	config, err := s.diagnoseAgentConfig(dir, session, sink) //nolint:contextcheck //own deadline.
	if err != nil {
		return nil, err
	}

	runlog.Infof(sink, "启动诊断 agent（%s），超时上限 %ds", config.Provider, config.TimeoutSeconds)

	raw, err := agent.RunRaw(ctx, config, diagnose.BuildPrompt(session), agentObserver(sink))
	if err != nil {
		runlog.Errorf(sink, "诊断 agent 运行失败：%v", err)

		return nil, fmt.Errorf("diagnose source %d: %w", sourceID, err)
	}

	report, err := diagnose.ParseReport(raw)
	if err != nil {
		runlog.Errorf(sink, "无法从返回中解析出诊断结论：%v", err)

		return nil, fmt.Errorf("diagnose source %d: %w", sourceID, err)
	}

	return s.buildDiagnoseReport(source, report, sink), nil //nolint:contextcheck //own deadline.
}

// diagnoseSession takes the snapshot one run works on, retrying the stored
// configuration first so the agent is told what failing looks like right now
// rather than what it looked like whenever the last scheduled fetch ran.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) diagnoseSession(source *feed.Source, sink runlog.Sink) *diagnose.Session {
	runlog.Infof(sink, "先用当前配置重试一次，确认失败是否还在")

	session := &diagnose.Session{
		SourceID:    source.ID,
		Source:      source,
		StoredError: source.ErrorInfo,
	}

	_, err := feed.ParseArticles(source, s.agentConfig(), sink)
	if err != nil {
		session.FreshError = err.Error()
	} else {
		runlog.Warnf(sink, "当前配置这次抓取成功了，可能是间歇性故障；仍然继续诊断，但不建议改动配置")
	}

	proxy, err := s.readHTTPProxy()
	if err == nil {
		session.HTTPProxy = proxy
	}

	return session
}

// diagnoseAgentConfig prepares the run directory and the agent configuration of
// one diagnosis: the session document the tool server reads, the mcp document
// the command line loads, and the tool set and time budget a diagnosis needs
// rather than the ones a fetch needs.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) diagnoseAgentConfig(dir string, session *diagnose.Session,
	sink runlog.Sink,
) (*agent.Config, error) {
	err := diagnose.WriteSession(dir, session)
	if err != nil {
		return nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate informer executable for the diagnose tools: %w", err)
	}

	mcpPath, err := diagnose.WriteMCPConfig(dir, executable, diagnose.ServeCommand, "--dir", dir)
	if err != nil {
		return nil, err
	}

	config := s.agentConfig()
	config.MCPConfigPath = mcpPath

	// the session's own tools, plus the read only web ones to explore with. What
	// keeps a diagnosis honest is not the absence of a search engine but the
	// rule in the prompt that the verdict rests on fetch_content's bytes.
	config.AllowedTools = diagnose.AllowedTools()

	if config.TimeoutSeconds < DiagnoseTimeoutSeconds {
		config.TimeoutSeconds = DiagnoseTimeoutSeconds
	}

	runlog.Infof(sink, "诊断工具已就绪：%s", diagnose.AllowedTools())

	return config, nil
}

// buildDiagnoseReport turns the agent's answer into a proposal informer has
// checked itself.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) buildDiagnoseReport(source *feed.Source, report *diagnose.Report,
	sink runlog.Sink,
) *DiagnoseReport {
	result := &DiagnoseReport{
		SourceID:          source.ID,
		AgentClaimedFixed: report.Fixed,
		Diagnosis:         report.Diagnosis,
		Advice:            report.Advice,
		Verification:      &DiagnoseVerification{},
	}

	runlog.Infof(sink, "诊断结论：%s", report.Diagnosis)

	result.Changes = report.Changes.Effective(source)
	if result.Changes == nil {
		if report.Advice != "" {
			runlog.Warnf(sink, "没有可用的配置改动，建议：%s", report.Advice)
		} else {
			runlog.Warnf(sink, "没有给出可用的配置改动")
		}

		return result
	}

	result.Diff = result.Changes.Diff(source)

	for _, change := range result.Diff {
		runlog.Infof(sink, "建议修改 %s：%q -> %q", change.Field, change.Old, change.New)
	}

	result.Verification = s.verifyCandidate(result.Changes.Apply(source), sink)
	result.Fixed = result.Verification.Error == "" && result.Verification.ArticleCount > 0

	if !result.Fixed && report.Fixed {
		runlog.Warnf(sink, "agent 声称已修复，但 informer 复核没有通过，因此不会推荐直接应用")
	}

	return result
}

// verifyCandidate parses a candidate configuration once more, for real.
//
// It is informer's own answer to "does this actually work", run in this process
// against the live page, and it is what the desktop shows above the apply
// button. The agent's claim is never the thing a person acts on.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) verifyCandidate(candidate *feed.Source, sink runlog.Sink) *ParseVerification {
	runlog.Infof(sink, "用建议的配置复核一次")

	verification := &ParseVerification{Ran: true}

	err := ValidateSource(candidate)
	if err != nil {
		verification.Error = err.Error()

		runlog.Errorf(sink, "复核失败，建议的配置本身不合法：%v", err)

		return verification
	}

	articles, err := feed.ParseArticles(candidate, s.agentConfig(), sink)
	if err != nil {
		verification.Error = err.Error()

		runlog.Errorf(sink, "复核失败：%v", err)

		return verification
	}

	verification.ArticleCount = len(articles)
	verification.Samples = articles

	if len(articles) > verifySampleArticles {
		verification.Samples = articles[:verifySampleArticles]
	}

	if len(articles) == 0 {
		runlog.Warnf(sink, "复核解析成功但一条都没有取到，不推荐应用")

		return verification
	}

	runlog.Infof(sink, "复核通过，解析出 %d 条", len(articles))

	return verification
}

// ApplySourceFix writes a diagnosis proposal into the stored source.
//
// This is the only call of the whole diagnosis path that writes, and it is
// reached only by a person pressing apply. It re-validates the result rather
// than trusting the proposal it was handed, then re-parses: a source that parses
// again is marked healthy on the spot, so the card stops claiming a failure the
// user just repaired.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) ApplySourceFix(sourceID int64, changes *diagnose.Changes, sink runlog.Sink) error {
	if changes.IsEmpty() {
		return fmt.Errorf("%w: the fix changes nothing", ErrInvalidArgument)
	}

	source, err := s.GetSource(sourceID)
	if err != nil {
		return err
	}

	patched := changes.Apply(source)

	err = ValidateSource(patched)
	if err != nil {
		return err
	}

	err = s.UpdateSource(patched)
	if err != nil {
		return err
	}

	for _, change := range changes.Diff(source) {
		runlog.Infof(sink, "已应用 %s：%q -> %q", change.Field, change.Old, change.New)
	}

	s.refreshSourceHealth(patched, sink)

	return nil
}

// refreshSourceHealth re-parses a just repaired source and records the outcome,
// so the subscription card reflects the repair instead of the failure that led
// to it. A failure here is recorded, not returned: the configuration was saved
// either way, and the user asked to save it.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) refreshSourceHealth(source *feed.Source, sink runlog.Sink) {
	s.ApplyHTTPProxy()

	_, err := feed.ParseArticles(source, s.agentConfig(), sink)
	if err != nil {
		runlog.Warnf(sink, "应用后再次抓取仍然失败：%v", err)

		s.markSourceStatus(source.ID, feed.StatusError, err.Error(), sink)

		return
	}

	runlog.Infof(sink, "应用后抓取成功，订阅状态已恢复正常")

	s.markSourceStatus(source.ID, feed.StatusNormal, "", sink)
}

// markSourceStatus records a fetch outcome on a source.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) markSourceStatus(sourceID int64, status int, errorInfo string, sink runlog.Sink) {
	err := s.db.Model(&feed.Source{}).Where("id = ?", sourceID).
		Updates(map[string]any{"status": status, "error_info": errorInfo}).Error
	if err != nil {
		runlog.Warnf(sink, "更新订阅状态失败：%v", err)
	}
}

// agentObserver adapts the agent's own progress notes onto the run log, so an
// agent run narrates itself exactly the way a test fetch does.
func agentObserver(sink runlog.Sink) agent.Observer {
	if sink == nil {
		return nil
	}

	return agent.ObserverFunc(func(level, text string) {
		runlog.Log(sink, level, strings.TrimSpace(text))
	})
}
