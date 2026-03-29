package commands

import "github.com/nicksnyder/go-i18n/v2/i18n"

var (
	MsgCmdShortDesc = &i18n.Message{
		ID:    "commands.CmdShortDesc",
		Other: "Financial Trading LLM AI Agent. **This is Not Financial Advice.**",
	}

	MsgGlobalOptsVerbosityDesc = &i18n.Message{
		ID:    "commands.GlobalOptsVerbosityDesc",
		Other: "Number for the log level verbosity (0, 1, or 2)",
	}
	MsgGlobalOptsDataRootDesc = &i18n.Message{
		ID:    "commands.GlobalOptsDataRootDesc",
		Other: "Path of data root directory",
	}
	MsgGlobalOptsLangDesc = &i18n.Message{
		ID:    "commands.GlobalOptsLangDesc",
		Other: "The language used in UI (en or zh)",
	}

	MsgCmdShortDescVersion = &i18n.Message{
		ID:    "commands.CmdShortDescVersion",
		Other: "Print the version information",
	}
	MsgVersionOptsOutputFormatDesc = &i18n.Message{
		ID:    "commands.VersionOptsOutputFormatDesc",
		Other: "Output format. One of (json)",
	}
)
