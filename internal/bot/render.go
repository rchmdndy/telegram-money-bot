package bot

import (
	"slices"
	"strconv"
	"strings"

	"github.com/rchmdndy/telegram-money-bot/internal/persona"
	"github.com/rchmdndy/telegram-money-bot/internal/storage"
)

// mainReplyKeyboard is the persistent reply keyboard of PRD §4.2. Its two
// right-hand buttons are shortcuts for /rekap and /settings.
func mainReplyKeyboard() Keyboard {
	return Keyboard{Reply: [][]string{
		{persona.BtnExpense, persona.BtnIncome},
		{persona.BtnRekap, persona.BtnSettings},
	}}
}

// dateKeyboard is step 2 of the add flow (PRD §4.2).
func dateKeyboard() Keyboard {
	return Keyboard{Inline: [][]InlineButton{{
		{Text: persona.BtnToday, Data: cbDateToday},
		{Text: persona.BtnYesterday, Data: cbDateYesterday},
		{Text: persona.BtnPickDate, Data: cbDatePick},
	}}}
}

// categoryKeyboard lays categories out two per row. back adds the
// `[⬅️ Kembali]` row that cancels the add flow (PRD §4.2).
func categoryKeyboard(cats []storage.Category, prefix string, back bool) Keyboard {
	rows := make([][]InlineButton, 0, len(cats)/2+2)
	for i := 0; i < len(cats); i += 2 {
		row := []InlineButton{categoryButton(cats[i], prefix)}
		if i+1 < len(cats) {
			row = append(row, categoryButton(cats[i+1], prefix))
		}
		rows = append(rows, row)
	}
	if back {
		rows = append(rows, []InlineButton{{Text: persona.BtnBack, Data: cbBack}})
	}
	return Keyboard{Inline: rows}
}

// categoryButton labels one category. An inactive one is marked, so a rename or
// deactivate picker cannot be mistaken for the add flow (PRD §4.4: inactive
// categories are hidden from input keyboards but still listable).
func categoryButton(c storage.Category, prefix string) InlineButton {
	text := c.Name
	if !c.Active {
		text += " " + persona.LabelInactive
	}
	return InlineButton{Text: text, Data: prefix + strconv.FormatInt(c.ID, 10)}
}

// confirmKeyboard is the confirmation card of the add and quick flows
// (PRD §4.2, §4.13.3).
func confirmKeyboard() Keyboard {
	return Keyboard{Inline: [][]InlineButton{{
		{Text: persona.BtnSave, Data: cbSave},
		{Text: persona.BtnNote, Data: cbNote},
		{Text: persona.BtnCancel, Data: cbCancel},
	}}}
}

// editKeyboard is the confirmation card of the edit flow (PRD §4.9).
func editKeyboard() Keyboard {
	return Keyboard{Inline: [][]InlineButton{{
		{Text: persona.BtnSave, Data: cbSave},
		{Text: persona.BtnCancel, Data: cbCancel},
	}}}
}

// editMenuKeyboard is the field menu of PRD §4.9. Every button carries the
// transaction being edited, so the menu itself needs no stored state.
func editMenuKeyboard(txID int64) Keyboard {
	id := strconv.FormatInt(txID, 10) + ":"
	btn := func(label, field string) InlineButton {
		return InlineButton{Text: label, Data: cbEditPrefix + id + field}
	}
	return Keyboard{Inline: [][]InlineButton{
		{btn(persona.BtnEditDate, fieldDate), btn(persona.BtnEditCategory, fieldCategory)},
		{btn(persona.BtnEditAmount, fieldAmount), btn(persona.BtnEditNote, fieldNote)},
		{btn(persona.BtnEditKind, fieldKind)},
		{btn(persona.BtnEditDone, fieldDone)},
	}}
}

// kindKeyboard picks the type of a new category or the new type of an edit
// (PRD §4.4, §4.9).
func kindKeyboard() Keyboard {
	return Keyboard{Inline: [][]InlineButton{{
		{Text: persona.BtnExpense, Data: cbKindPrefix + string(storage.KindExpense)},
		{Text: persona.BtnIncome, Data: cbKindPrefix + string(storage.KindIncome)},
	}}}
}

// categoryMenuKeyboard is the /kategori menu (PRD §4.4).
func categoryMenuKeyboard() Keyboard {
	return Keyboard{Inline: [][]InlineButton{{
		{Text: persona.BtnCatAdd, Data: cbCatMenuAdd},
		{Text: persona.BtnCatRename, Data: cbCatMenuRename},
		{Text: persona.BtnCatOff, Data: cbCatMenuOff},
	}}}
}

// personaKeyboard lists the four personas, marking the active one (PRD §4.12).
func personaKeyboard(active persona.ID) Keyboard {
	ids := persona.IDs()
	labels := persona.Labels()
	rows := make([][]InlineButton, 0, (len(ids)+1)/2)
	for i := 0; i < len(ids); i += 2 {
		row := make([]InlineButton, 0, 2)
		for j := i; j < len(ids) && j < i+2; j++ {
			text := labels[ids[j]]
			if ids[j] == active {
				text = persona.LabelItemPrefix + text
			}
			row = append(row, InlineButton{Text: text, Data: cbPersonaPrefix + string(ids[j])})
		}
		rows = append(rows, row)
	}
	return Keyboard{Inline: rows}
}

// transactionKeyboard builds the `[✏️][🗑]` row of every transaction line of
// /hari and /terakhir (PRD §4.9). Telegram draws an inline keyboard directly
// under the message text, so the rows are reversed to line each button up with
// its own transaction.
func transactionKeyboard(rows []storage.TransactionRow) Keyboard {
	kb := make([][]InlineButton, 0, len(rows))
	for _, r := range rows {
		id := strconv.FormatInt(r.ID, 10)
		kb = append(kb, []InlineButton{
			{Text: persona.BtnEdit, Data: cbTxEditPrefix + id},
			{Text: persona.BtnDelete, Data: cbTxDeletePrefix + id},
		})
	}
	slices.Reverse(kb)
	return Keyboard{Inline: kb}
}

// noteOrDash renders an empty note as the persona placeholder.
func noteOrDash(note string) string {
	if strings.TrimSpace(note) == "" {
		return persona.LabelNoNote
	}
	return note
}

// kindWord is the lowercase kind word the confirmation headers use.
func kindWord(kind string) string {
	if storage.Kind(kind) == storage.KindIncome {
		return persona.KindIncomeWord
	}
	return persona.KindExpenseWord
}

// editWord is the {kind} token of tx.edit.confirm.header: the CHANGED FIELD,
// not the transaction kind (PRD §4.9: `Ubah nominal?`).
func editWord(field string) string {
	if w, ok := editFieldWords[field]; ok {
		return w
	}
	return persona.FieldWordAmount
}
