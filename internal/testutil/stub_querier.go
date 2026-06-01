// Package testutil provides test helpers for the VIGIL application.
package testutil

import (
	"context"

	"github.com/3lbits/vigil/internal/db"
	"github.com/google/uuid"
)

// StubQuerier implements db.Querier with zero-value returns.
// Embed it in a test-local struct and override only the methods under test.
type StubQuerier struct{}

func (StubQuerier) AddParticipant(_ context.Context, _ db.AddParticipantParams) error { return nil }
func (StubQuerier) IsParticipant(_ context.Context, _ db.IsParticipantParams) (bool, error) {
	return false, nil
}
func (StubQuerier) CompleteActivity(_ context.Context, _ db.CompleteActivityParams) (db.Activity, error) {
	return db.Activity{}, nil
}
func (StubQuerier) CreateActivity(_ context.Context, _ db.CreateActivityParams) (db.Activity, error) {
	return db.Activity{}, nil
}
func (StubQuerier) DeleteActivity(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) GetActivity(_ context.Context, _ uuid.UUID) (db.GetActivityRow, error) {
	return db.GetActivityRow{}, nil
}
func (StubQuerier) FilterActivities(_ context.Context, _ db.FilterActivitiesParams) ([]db.FilterActivitiesRow, error) {
	return nil, nil
}
func (StubQuerier) FilterMeasures(_ context.Context, _ db.FilterMeasuresParams) ([]db.Measure, error) {
	return nil, nil
}
func (StubQuerier) ListActivities(_ context.Context) ([]db.ListActivitiesRow, error) {
	return nil, nil
}
func (StubQuerier) ListRecentActivities(_ context.Context) ([]db.ListRecentActivitiesRow, error) {
	return nil, nil
}
func (StubQuerier) MarkActivityInProgress(_ context.Context, _ uuid.UUID) (db.Activity, error) {
	return db.Activity{}, nil
}
func (StubQuerier) ReopenActivity(_ context.Context, _ uuid.UUID) (db.Activity, error) {
	return db.Activity{}, nil
}
func (StubQuerier) MarkOverdueActivities(_ context.Context) error { return nil }
func (StubQuerier) UpdateActivity(_ context.Context, _ db.UpdateActivityParams) (db.Activity, error) {
	return db.Activity{}, nil
}
func (StubQuerier) UpdateMeasureLastVerified(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) CountCoveredRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (StubQuerier) CountRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (StubQuerier) CreateFramework(_ context.Context, _ db.CreateFrameworkParams) (db.Framework, error) {
	return db.Framework{}, nil
}
func (StubQuerier) CreateMeasure(_ context.Context, _ db.CreateMeasureParams) (db.Measure, error) {
	return db.Measure{}, nil
}
func (StubQuerier) CreateRequirement(_ context.Context, _ db.CreateRequirementParams) (db.Requirement, error) {
	return db.Requirement{}, nil
}
func (StubQuerier) DeleteFramework(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) DeleteMeasure(_ context.Context, _ uuid.UUID) error   { return nil }
func (StubQuerier) DeleteRequirement(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (StubQuerier) GetDashboardStats(_ context.Context) (db.GetDashboardStatsRow, error) {
	return db.GetDashboardStatsRow{}, nil
}
func (StubQuerier) GetFramework(_ context.Context, _ uuid.UUID) (db.Framework, error) {
	return db.Framework{}, nil
}
func (StubQuerier) GetMeasure(_ context.Context, _ uuid.UUID) (db.Measure, error) {
	return db.Measure{}, nil
}
func (StubQuerier) GetRequirement(_ context.Context, _ uuid.UUID) (db.Requirement, error) {
	return db.Requirement{}, nil
}
func (StubQuerier) ClaimPendingUser(_ context.Context, _ db.ClaimPendingUserParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) DeleteUser(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) GetUserByEmail(_ context.Context, _ string) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) GetUserByID(_ context.Context, _ uuid.UUID) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) PreCreateUser(_ context.Context, _ db.PreCreateUserParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) LinkMeasureToRequirement(_ context.Context, _ db.LinkMeasureToRequirementParams) error {
	return nil
}
func (StubQuerier) ListFrameworks(_ context.Context) ([]db.Framework, error) { return nil, nil }
func (StubQuerier) ListMeasureFrameworkLinks(_ context.Context) ([]db.ListMeasureFrameworkLinksRow, error) {
	return nil, nil
}
func (StubQuerier) ListMeasures(_ context.Context) ([]db.Measure, error) { return nil, nil }
func (StubQuerier) ListMeasuresForRequirement(_ context.Context, _ uuid.UUID) ([]db.ListMeasuresForRequirementRow, error) {
	return nil, nil
}
func (StubQuerier) ListRequirementsByFramework(_ context.Context, _ uuid.UUID) ([]db.Requirement, error) {
	return nil, nil
}
func (StubQuerier) ListRequirementsForMeasure(_ context.Context, _ uuid.UUID) ([]db.ListRequirementsForMeasureRow, error) {
	return nil, nil
}
func (StubQuerier) ListUsers(_ context.Context) ([]db.User, error)        { return nil, nil }
func (StubQuerier) ListDevStubUsers(_ context.Context) ([]db.User, error) { return nil, nil }
func (StubQuerier) SetUserRole(_ context.Context, _ db.SetUserRoleParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) UnlinkMeasureFromRequirement(_ context.Context, _ db.UnlinkMeasureFromRequirementParams) error {
	return nil
}
func (StubQuerier) UpdateFramework(_ context.Context, _ db.UpdateFrameworkParams) (db.Framework, error) {
	return db.Framework{}, nil
}
func (StubQuerier) UpdateMeasure(_ context.Context, _ db.UpdateMeasureParams) (db.Measure, error) {
	return db.Measure{}, nil
}
func (StubQuerier) UpdateRequirement(_ context.Context, _ db.UpdateRequirementParams) (db.Requirement, error) {
	return db.Requirement{}, nil
}
func (StubQuerier) UpsertUser(_ context.Context, _ db.UpsertUserParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) UpsertDevStubUser(_ context.Context, _ db.UpsertDevStubUserParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) InsertAuditLog(_ context.Context, _ db.InsertAuditLogParams) error { return nil }
func (StubQuerier) ListAuditLogForMeasure(_ context.Context, _ string) ([]db.ListAuditLogForMeasureRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForActivity(_ context.Context, _ string) ([]db.ListAuditLogForActivityRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForRequirement(_ context.Context, _ string) ([]db.ListAuditLogForRequirementRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForFramework(_ context.Context, _ string) ([]db.ListAuditLogForFrameworkRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForAsset(_ context.Context, _ string) ([]db.ListAuditLogForAssetRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForRisk(_ context.Context, _ string) ([]db.ListAuditLogForRiskRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogForAssessment(_ context.Context, _ string) ([]db.ListAuditLogForAssessmentRow, error) {
	return nil, nil
}
func (StubQuerier) ListAuditLogAdmin(_ context.Context) ([]db.ListAuditLogAdminRow, error) {
	return nil, nil
}
func (StubQuerier) ListAllRequirements(_ context.Context) ([]db.ListAllRequirementsRow, error) {
	return nil, nil
}
func (StubQuerier) ListActiveSessionsByUser(_ context.Context) ([]db.ListActiveSessionsByUserRow, error) {
	return nil, nil
}
func (StubQuerier) DeleteSessionsByUserID(_ context.Context, _ uuid.NullUUID) error { return nil }
func (StubQuerier) AddMeasureLink(_ context.Context, _ db.AddMeasureLinkParams) (db.MeasureLink, error) {
	return db.MeasureLink{}, nil
}
func (StubQuerier) DeleteMeasureLink(_ context.Context, _ db.DeleteMeasureLinkParams) error {
	return nil
}
func (StubQuerier) ListMeasureLinks(_ context.Context, _ uuid.UUID) ([]db.MeasureLink, error) {
	return nil, nil
}
func (StubQuerier) ListActivitiesForMeasure(_ context.Context, _ uuid.NullUUID) ([]db.ListActivitiesForMeasureRow, error) {
	return nil, nil
}
func (StubQuerier) ListActivitiesForUser(_ context.Context, _ uuid.NullUUID) ([]db.ListActivitiesForUserRow, error) {
	return nil, nil
}
func (StubQuerier) ListMeasuresForUser(_ context.Context, _ uuid.NullUUID) ([]db.Measure, error) {
	return nil, nil
}
func (StubQuerier) ListOwnedActivities(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedActivitiesRow, error) {
	return nil, nil
}
func (StubQuerier) ListOwnedMeasures(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedMeasuresRow, error) {
	return nil, nil
}
func (StubQuerier) ListOwnedRisks(_ context.Context, _ uuid.NullUUID) ([]db.ListOwnedRisksRow, error) {
	return nil, nil
}
func (StubQuerier) CountRisksForMatrix(_ context.Context) ([]db.CountRisksForMatrixRow, error) {
	return nil, nil
}
func (StubQuerier) CreateRisk(_ context.Context, _ db.CreateRiskParams) (db.Risk, error) {
	return db.Risk{}, nil
}
func (StubQuerier) CreateRiskAssessment(_ context.Context, _ db.CreateRiskAssessmentParams) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) DeleteRisk(_ context.Context, _ uuid.UUID) error           { return nil }
func (StubQuerier) DeleteRiskAssessment(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) GetRisk(_ context.Context, _ uuid.UUID) (db.Risk, error)   { return db.Risk{}, nil }
func (StubQuerier) GetRiskAssessment(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) GetRiskStats(_ context.Context) (db.GetRiskStatsRow, error) {
	return db.GetRiskStatsRow{}, nil
}
func (StubQuerier) LinkRiskToMeasure(_ context.Context, _ db.LinkRiskToMeasureParams) error {
	return nil
}
func (StubQuerier) CreateRiskReassessmentEvent(_ context.Context, _ db.CreateRiskReassessmentEventParams) error {
	return nil
}
func (StubQuerier) FlagRiskForReview(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) ListMeasuresForRisk(_ context.Context, _ uuid.UUID) ([]db.Measure, error) {
	return nil, nil
}
func (StubQuerier) ListRiskReassessmentEvents(_ context.Context, _ db.ListRiskReassessmentEventsParams) ([]db.ListRiskReassessmentEventsRow, error) {
	return nil, nil
}
func (StubQuerier) ListRiskReviewQueue(_ context.Context) ([]db.ListRiskReviewQueueRow, error) {
	return nil, nil
}
func (StubQuerier) ListRiskReviewQueueForUser(_ context.Context, _ uuid.UUID) ([]db.ListRiskReviewQueueForUserRow, error) {
	return nil, nil
}
func (StubQuerier) ListRiskAssessments(_ context.Context) ([]db.RiskAssessment, error) {
	return nil, nil
}
func (StubQuerier) ListRiskAssessmentsForUser(_ context.Context, _ uuid.NullUUID) ([]db.RiskAssessment, error) {
	return nil, nil
}
func (StubQuerier) ListRisksForAssessment(_ context.Context, _ uuid.UUID) ([]db.Risk, error) {
	return nil, nil
}
func (StubQuerier) ListRisksForMeasure(_ context.Context, _ uuid.UUID) ([]db.Risk, error) {
	return nil, nil
}
func (StubQuerier) ListMeasureRiskLinkIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (StubQuerier) ListTopRisks(_ context.Context) ([]db.ListTopRisksRow, error) { return nil, nil }
func (StubQuerier) ListAllRisks(_ context.Context) ([]db.ListAllRisksRow, error) { return nil, nil }
func (StubQuerier) SearchMeasures(_ context.Context, _ string) ([]db.Measure, error) {
	return nil, nil
}
func (StubQuerier) UnlinkRiskFromMeasure(_ context.Context, _ db.UnlinkRiskFromMeasureParams) error {
	return nil
}
func (StubQuerier) UpdateRiskAssessmentReviewed(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) UpdateRiskAssessmentStep(_ context.Context, _ db.UpdateRiskAssessmentStepParams) error {
	return nil
}
func (StubQuerier) UpdateRiskAssessmentStep1(_ context.Context, _ db.UpdateRiskAssessmentStep1Params) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) UpdateRiskDecision(_ context.Context, _ db.UpdateRiskDecisionParams) error {
	return nil
}
func (StubQuerier) UpdateRiskIdentification(_ context.Context, _ db.UpdateRiskIdentificationParams) (db.Risk, error) {
	return db.Risk{}, nil
}
func (StubQuerier) UpdateRiskCurrentScores(_ context.Context, _ db.UpdateRiskCurrentScoresParams) (db.Risk, error) {
	return db.Risk{}, nil
}
func (StubQuerier) ReassessRiskCurrentScores(_ context.Context, _ db.ReassessRiskCurrentScoresParams) (db.Risk, error) {
	return db.Risk{}, nil
}
func (StubQuerier) UpdateRiskTargetScore(_ context.Context, _ db.UpdateRiskTargetScoreParams) error {
	return nil
}
func (StubQuerier) CreateOrganization(_ context.Context, _ db.CreateOrganizationParams) (db.Organization, error) {
	return db.Organization{}, nil
}
func (StubQuerier) DeleteOrganization(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) GetOrganization(_ context.Context, _ uuid.UUID) (db.Organization, error) {
	return db.Organization{}, nil
}
func (StubQuerier) GetRiskGlobalSettings(_ context.Context) (db.RiskGlobalSetting, error) {
	return db.RiskGlobalSetting{}, nil
}
func (StubQuerier) ListRiskScaleLabels(_ context.Context) ([]db.RiskScaleLabel, error) {
	return nil, nil
}
func (StubQuerier) UpsertRiskScaleLabel(_ context.Context, _ db.UpsertRiskScaleLabelParams) error {
	return nil
}
func (StubQuerier) ListParticipantsForAssessment(_ context.Context, _ uuid.UUID) ([]db.User, error) {
	return nil, nil
}
func (StubQuerier) ClearAssessmentParticipants(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) AddAssessmentParticipant(_ context.Context, _ db.AddAssessmentParticipantParams) error {
	return nil
}
func (StubQuerier) AddAssetToAssessment(_ context.Context, _ db.AddAssetToAssessmentParams) error {
	return nil
}
func (StubQuerier) CreateAsset(_ context.Context, _ db.CreateAssetParams) (db.Asset, error) {
	return db.Asset{}, nil
}
func (StubQuerier) DeleteAsset(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) GetAsset(_ context.Context, _ uuid.UUID) (db.Asset, error) {
	return db.Asset{}, nil
}
func (StubQuerier) ListAssets(_ context.Context, _ db.ListAssetsParams) ([]db.Asset, error) {
	return nil, nil
}
func (StubQuerier) ListAssetsForAssessment(_ context.Context, _ uuid.UUID) ([]db.Asset, error) {
	return nil, nil
}
func (StubQuerier) RemoveAssessmentParticipant(_ context.Context, _ db.RemoveAssessmentParticipantParams) error {
	return nil
}
func (StubQuerier) RemoveAssetFromAssessment(_ context.Context, _ db.RemoveAssetFromAssessmentParams) error {
	return nil
}
func (StubQuerier) LinkRiskToActivity(_ context.Context, _ db.LinkRiskToActivityParams) error {
	return nil
}
func (StubQuerier) LinkRiskToAsset(_ context.Context, _ db.LinkRiskToAssetParams) error {
	return nil
}
func (StubQuerier) ListActivitiesForRisk(_ context.Context, _ uuid.UUID) ([]db.Activity, error) {
	return nil, nil
}
func (StubQuerier) ListAssetsForRisk(_ context.Context, _ uuid.UUID) ([]db.Asset, error) {
	return nil, nil
}
func (StubQuerier) ListRisksForActivity(_ context.Context, _ uuid.UUID) ([]db.Risk, error) {
	return nil, nil
}
func (StubQuerier) ListOrganizations(_ context.Context) ([]db.Organization, error) { return nil, nil }
func (StubQuerier) SearchActivities(_ context.Context, _ string) ([]db.Activity, error) {
	return nil, nil
}
func (StubQuerier) SearchAssetsForRisk(_ context.Context, _ db.SearchAssetsForRiskParams) ([]db.Asset, error) {
	return nil, nil
}
func (StubQuerier) SearchAssetsToLink(_ context.Context, _ db.SearchAssetsToLinkParams) ([]db.Asset, error) {
	return nil, nil
}
func (StubQuerier) UnlinkRiskFromActivity(_ context.Context, _ db.UnlinkRiskFromActivityParams) error {
	return nil
}
func (StubQuerier) UnlinkRiskFromAsset(_ context.Context, _ db.UnlinkRiskFromAssetParams) error {
	return nil
}
func (StubQuerier) ClearRiskAssets(_ context.Context, _ uuid.UUID) error { return nil }
func (StubQuerier) UpdateRiskGlobalSettings(_ context.Context, _ db.UpdateRiskGlobalSettingsParams) error {
	return nil
}
func (StubQuerier) UpdateAsset(_ context.Context, _ db.UpdateAssetParams) (db.Asset, error) {
	return db.Asset{}, nil
}
func (StubQuerier) AcceptAssessment(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) DeclineAssessment(_ context.Context, _ db.DeclineAssessmentParams) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) SetUserOrg(_ context.Context, _ db.SetUserOrgParams) (db.User, error) {
	return db.User{}, nil
}
func (StubQuerier) GetAppSettings(_ context.Context) (db.AppSetting, error) {
	return db.AppSetting{
		ID:                1,
		ComplianceEnabled: true,
		RiskEnabled:       true,
		ActivitiesEnabled: true,
		AssetsEnabled:     true,
		PlaygroundEnabled: true,
		AvvikEnabled:      true,
	}, nil
}
func (StubQuerier) UpdateAppSettings(_ context.Context, _ db.UpdateAppSettingsParams) error {
	return nil
}
func (StubQuerier) IsRiskAssessmentAccessible(_ context.Context, _ db.IsRiskAssessmentAccessibleParams) (bool, error) {
	return true, nil
}
func (StubQuerier) IsRiskAssessmentParticipant(_ context.Context, _ db.IsRiskAssessmentParticipantParams) (bool, error) {
	return false, nil
}
func (StubQuerier) ToggleRiskAssessmentPublic(_ context.Context, _ uuid.UUID) (db.RiskAssessment, error) {
	return db.RiskAssessment{}, nil
}
func (StubQuerier) ListAllRisksForUser(_ context.Context, _ uuid.UUID) ([]db.ListAllRisksForUserRow, error) {
	return nil, nil
}
func (StubQuerier) AddAvvikAttachment(_ context.Context, _ db.AddAvvikAttachmentParams) (db.AvvikAttachment, error) {
	return db.AvvikAttachment{}, nil
}
func (StubQuerier) AddAvvikEvent(_ context.Context, _ db.AddAvvikEventParams) (db.AvvikEvent, error) {
	return db.AvvikEvent{}, nil
}
func (StubQuerier) AddAvvikNotification(_ context.Context, _ db.AddAvvikNotificationParams) (db.AvvikNotification, error) {
	return db.AvvikNotification{}, nil
}
func (StubQuerier) CreateAvvik(_ context.Context, _ db.CreateAvvikParams) (db.Avvik, error) {
	return db.Avvik{}, nil
}
func (StubQuerier) GetAvvik(_ context.Context, _ uuid.UUID) (db.Avvik, error) { return db.Avvik{}, nil }
func (StubQuerier) LinkAvvikActivity(_ context.Context, _ db.LinkAvvikActivityParams) error {
	return nil
}
func (StubQuerier) LinkAvvikMeasure(_ context.Context, _ db.LinkAvvikMeasureParams) error {
	return nil
}
func (StubQuerier) ListAvvik(_ context.Context, _ db.ListAvvikParams) ([]db.Avvik, error) {
	return nil, nil
}
func (StubQuerier) ListAvvikActivities(_ context.Context, _ uuid.UUID) ([]db.Activity, error) {
	return nil, nil
}
func (StubQuerier) ListAvvikAttachments(_ context.Context, _ uuid.UUID) ([]db.AvvikAttachment, error) {
	return nil, nil
}
func (StubQuerier) ListAvvikEvents(_ context.Context, _ uuid.UUID) ([]db.AvvikEvent, error) {
	return nil, nil
}
func (StubQuerier) ListAvvikMeasures(_ context.Context, _ uuid.UUID) ([]db.Measure, error) {
	return nil, nil
}
func (StubQuerier) ListAvvikNotifications(_ context.Context, _ uuid.UUID) ([]db.AvvikNotification, error) {
	return nil, nil
}
func (StubQuerier) UnlinkAvvikActivity(_ context.Context, _ db.UnlinkAvvikActivityParams) error {
	return nil
}
func (StubQuerier) UnlinkAvvikMeasure(_ context.Context, _ db.UnlinkAvvikMeasureParams) error {
	return nil
}
func (StubQuerier) UpdateAvvikClosureFlags(_ context.Context, _ db.UpdateAvvikClosureFlagsParams) (db.Avvik, error) {
	return db.Avvik{}, nil
}
func (StubQuerier) UpdateAvvikStatus(_ context.Context, _ db.UpdateAvvikStatusParams) (db.Avvik, error) {
	return db.Avvik{}, nil
}
func (StubQuerier) UpdateAvvikTriage(_ context.Context, _ db.UpdateAvvikTriageParams) (db.Avvik, error) {
	return db.Avvik{}, nil
}
