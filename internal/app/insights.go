package app

import (
	"github.com/lawrips/tkt/internal/engine"
)

const (
	defaultTimelineWeeks = 4
	maxTimelineWeeks     = 260
)

type StatsReport struct {
	Total       int            `json:"total"`
	ByStatus    map[string]int `json:"by_status"`
	ByType      map[string]int `json:"by_type"`
	ByPriority  map[int]int    `json:"by_priority"`
	Ready       int            `json:"ready"`
	Blocked     int            `json:"blocked"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
}

type TimelineReport struct {
	Weeks       []engine.TimelineWeek `json:"weeks"`
	Diagnostics []Diagnostic          `json:"diagnostics,omitempty"`
}

type EpicOverviewReport struct {
	Epics       []engine.EpicProgress `json:"epics"`
	Diagnostics []Diagnostic          `json:"diagnostics,omitempty"`
}

func (s *Service) Stats(projectName string) (StatsReport, error) {
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return StatsReport{}, err
	}
	records, diagnostics, err := LoadTicketsWithDiagnostics(info.TicketDir)
	if err != nil {
		return StatsReport{}, err
	}
	stats := engine.ComputeStats(records)
	return StatsReport{
		Total:       stats.Total,
		ByStatus:    stats.ByStatus,
		ByType:      stats.ByType,
		ByPriority:  stats.ByPriority,
		Ready:       stats.Ready,
		Blocked:     stats.Blocked,
		Diagnostics: diagnostics,
	}, nil
}

func (s *Service) Timeline(projectName string, weeks int) (TimelineReport, error) {
	if weeks <= 0 {
		weeks = defaultTimelineWeeks
	}
	if weeks > maxTimelineWeeks {
		return TimelineReport{}, validationErrorf("weeks must be between 1 and %d", maxTimelineWeeks)
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TimelineReport{}, err
	}
	records, diagnostics, err := LoadTicketsWithDiagnostics(info.TicketDir)
	if err != nil {
		return TimelineReport{}, err
	}
	return TimelineReport{
		Weeks:       engine.ClosedByWeek(records, weeks, s.now()),
		Diagnostics: diagnostics,
	}, nil
}

func (s *Service) EpicOverview(projectName string) (EpicOverviewReport, error) {
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return EpicOverviewReport{}, err
	}
	records, diagnostics, err := LoadTicketsWithDiagnostics(info.TicketDir)
	if err != nil {
		return EpicOverviewReport{}, err
	}
	return EpicOverviewReport{
		Epics:       engine.ComputeEpicProgress(records),
		Diagnostics: diagnostics,
	}, nil
}
