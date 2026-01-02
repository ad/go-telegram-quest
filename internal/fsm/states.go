package fsm

const (
	StateInit           = "init"
	StateWelcome        = "welcome"
	StateAwaitingAnswer = "awaiting_answer"
	StateWaitingReview  = "waiting_review"
	StateCompleted      = "completed"

	StateAdminMenu          = "admin_menu"
	StateAdminAddStep       = "admin_add_step"
	StateAdminEditStep      = "admin_edit_step"
	StateAdminManageAnswers = "admin_manage_answers"
	StateAdminEditSettings  = "admin_edit_settings"

	StateAdminAddStepText      = "admin_add_step_text"
	StateAdminAddStepType      = "admin_add_step_type"
	StateAdminAddStepImages    = "admin_add_step_images"
	StateAdminAddStepAnswers   = "admin_add_step_answers"
	StateAdminEditStepText     = "admin_edit_step_text"
	StateAdminEditStepImages   = "admin_edit_step_images"
	StateAdminAddAnswer        = "admin_add_answer"
	StateAdminDeleteAnswer     = "admin_delete_answer"
	StateAdminEditSettingValue = "admin_edit_setting_value"
)
