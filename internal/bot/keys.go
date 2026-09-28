package bot

import "github.com/dandy/telegram_money_bot/internal/persona"

// Callback data carried by every inline button. Telegram only guarantees
// 64 bytes, so the payload stays short: a prefix plus an ID.
const (
	cbDateToday     = "dt:today"
	cbDateYesterday = "dt:yesterday"
	cbDatePick      = "dt:pick"

	cbSave   = "save"
	cbNote   = "note"
	cbCancel = "cancel"
	cbBack   = "back"

	cbTxEditPrefix    = "tx:e:"
	cbTxDeletePrefix  = "tx:d:"
	cbCatPickPrefix   = "cat:" // cat:<category_id>
	cbCatRenamePrefix = "cr:"  // cr:<category_id>
	cbCatOffPrefix    = "co:"  // co:<category_id>
	cbKindPrefix      = "ck:"  // ck:expense / ck:income
	cbPersonaPrefix   = "p:"   // p:<persona_id>
	cbEditPrefix      = "ed:"  // ed:<field>

	cbCatMenuAdd    = "kc:add"
	cbCatMenuRename = "kc:rename"
	cbCatMenuOff    = "kc:off"
)

// edit fields carried by cbEditPrefix.
const (
	fieldDate     = "date"
	fieldCategory = "category"
	fieldAmount   = "amount"
	fieldNote     = "note"
	fieldKind     = "kind"
	fieldDone     = "done"
)

// editFieldWords maps an edit field to the persona.FieldWord* label used by
// tx.edit.confirm.header ("Ubah nominal?"), per PRD §4.9.
var editFieldWords = map[string]string{
	fieldDate:     persona.FieldWordDate,
	fieldCategory: persona.FieldWordCategory,
	fieldAmount:   persona.FieldWordAmount,
	fieldNote:     persona.FieldWordNote,
	fieldKind:     persona.FieldWordKind,
}
