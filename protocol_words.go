package venom

// The Russian words of a protocol, in one place: the writer prints them and the tests of the fork compare against these
// names, so no test carries Cyrillic (owner 2026-11-15: «fork venom tests must be only in English»).
const (
	protocolHeadRight      = "## Правильное поведение"
	protocolHeadWrong      = "## Что неправильно"
	protocolHeadPresence   = "## Наличие данных"
	protocolHeadRequest    = "## Запрос"
	protocolHeadAnswer     = "## Ответ"
	protocolHeadAfter      = "## Состояние после вызова"
	protocolHeadTrace      = "## Трасса"
	protocolHeadVerdict    = "## Вердикт"
	protocolPassed         = "✅ пройден"
	protocolFailed         = "❌ не пройден"
	protocolSkipped        = "⏭ пропущен"
	protocolWordPresent    = "есть"
	protocolWordAbsent     = "нет"
	protocolDataPresent    = "данные есть"
	protocolDataAbsent     = "данных нет"
	protocolNotConfirmed   = "❌ не подтверждено"
	protocolNoPrecondition = "предусловия нет"
	protocolPresenceHeader = "| сущность | ключ | запрос | есть/нет | trace id | ключ в проверяемом вызове |"
	protocolKeySent        = "✅ отправлен"
	protocolKeyNotSent     = "❌ не отправлен"
	protocolStepLine       = "- шаг: %d «%s»"
	protocolAfterStep      = "### шаг %d «%s» %s"
	protocolMethodLine     = "- метод: "
	protocolMasked         = "*** (%d знаков)"
	protocolNoTrace        = "трассы нет"
	protocolNoTree         = "- дерева нет:"
)
