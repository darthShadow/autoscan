package logmsg

type event struct{}

func (event) Msg(string)          {}
func (event) Msgf(string, ...any) {}

func messages(value string) {
	var log event
	log.Msg("receiver-request")   // want `Log message must be Title Case words, not a hyphenated token`
	log.Msg("Detected-change")    // want `Log message must be Title Case words, not a hyphenated token`
	log.Msg("would-delete")       // want `Log message must be Title Case words, not a hyphenated token`
	log.Msgf("cleanup-%s", value) // want `Log message must be Title Case words, not a hyphenated token`
	log.Msg(`Detected-change`)    // want `Log message must be Title Case words, not a hyphenated token`
	log.Msg("Detected Change")
	log.Msg("Re-Constructing Path After ESTALE")
	log.Msg("Processing video")
	log.Msg("processing video") // want `Log message should start with capital letter`
}

