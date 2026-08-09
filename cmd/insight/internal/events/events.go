package events

// HookEvents returns all Claude Code hook event types (CamelCase).
func HookEvents() []string {
	return []string{
		"SessionStart",
		"SessionEnd",
		"Setup",
		"UserPromptSubmit",
		"UserPromptExpansion",
		"PreToolUse",
		"PostToolUse",
		"PostToolUseFailure",
		"PostToolBatch",
		"PermissionRequest",
		"PermissionDenied",
		"Notification",
		"MessageDisplay",
		"SubagentStart",
		"SubagentStop",
		"TaskCreated",
		"TaskCompleted",
		"Stop",
		"StopFailure",
		"TeammateIdle",
		"InstructionsLoaded",
		"ConfigChange",
		"CwdChanged",
		"DirectoryAdded",
		"FileChanged",
		"WorktreeCreate",
		"WorktreeRemove",
		"PreCompact",
		"PostCompact",
		"Elicitation",
		"ElicitationResult",
	}
}
