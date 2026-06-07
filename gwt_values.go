package gocronometer

// The following constants contain header values required for GWT requests. These values are found by inspecting a
// request from the web app. When the web app is updated these values can change. The values provided here are the
// default that will be used by the library if new values are not provided.
const (
	GWTContentType = "text/x-gwt-rpc; charset=UTF-8"
	GWTModuleBase  = "https://cronometer.com/cronometer/"
	GWTPermutation = "3372E4AA1F74FD0D8D7465343215E6AE"

	// GWTHeader is what appears to be a hash value that is provided at the beginning of every GWT request. As it
	// changes with app updates it appears to be related to validating the version the requester is expecting.
	//GWTHeader = "3B6C5196158464C5643BA376AF05E7F1"
	GWTHeader = "08048AF8BA7E897E74754A658DF1BEC5"
)

// The following are the GWT procedure calls as found from inspection of the app.
const (

	// GWTGenerateAuthToken will generate a GWT auth token. The only known use case is for accessing non GWT API calls
	// such as data export.
	// The first parameter in the string should be the sesnonce and the second is the users ID.
	GWTGenerateAuthToken = "7|0|8|https://cronometer.com/cronometer/|" + GWTHeader + "|com.cronometer.shared.rpc.CronometerService|generateAuthorizationToken" +
		"|java.lang.String/2004016611|I|com.cronometer.shared.user.AuthScope/2065601159|%s|1|2|3|4|4|5|6|6|7|8|%s|3600|7|2|"

	// GWTAuthenticate will authenticate with the GWT api. The sesnonce should be set in the cookies.
	GWTAuthenticate = "7|0|5|https://cronometer.com/cronometer/|" + GWTHeader + "|com.cronometer.shared.rpc.CronometerService|authenticate|java.lang.Integer/3438268394|1|2|3|4|1|5|5|-300|"

	// GWTLogout will log the session out.
	// The only parameter should be the sesnonce.
	GWTLogout = "7|0|6|https://cronometer.com/cronometer/|" + GWTHeader + "|com.cronometer.shared.rpc.CronometerService|logout|java.lang.String/2004016611|%s|1|2|3|4|1|5|6|"

	// GWTUpdateDiaryAddServing adds a serving to the diary without a time.
	// Parameters: header, sesnonce, user ID, day, month, year, group ID, amount, food ID, measure ID.
	GWTUpdateDiaryAddServing = "7|0|12|https://cronometer.com/cronometer/|%s|com.cronometer.shared.rpc.CronometerService|updateDiary|java.lang.String/2004016611|I|java.util.List|%s|java.util.Collections$SingletonList/1586180994|com.cronometer.shared.entries.changes.AddEntryChange/3949104564|com.cronometer.shared.entries.models.Serving/2553599101|com.cronometer.shared.entries.models.Day/782579793|1|2|3|4|3|5|6|7|8|%s|9|10|1|1|11|12|%d|%d|%d|1|1|0|%d|0|0|%s|%d|A|%d|0|0|"

	// GWTUpdateDiaryAddServingWithTime adds a serving to the diary with a time.
	// Parameters: header, sesnonce, user ID, day, month, year, group ID, hour, minute, user ID, amount, food ID, measure ID.
	GWTUpdateDiaryAddServingWithTime = "7|0|13|https://cronometer.com/cronometer/|%s|com.cronometer.shared.rpc.CronometerService|updateDiary|java.lang.String/2004016611|I|java.util.List|%s|java.util.Collections$SingletonList/1586180994|com.cronometer.shared.entries.changes.AddEntryChange/3949104564|com.cronometer.shared.entries.models.Serving/2553599101|com.cronometer.shared.entries.models.Day/782579793|com.cronometer.shared.entries.models.Time/1552252503|1|2|3|4|3|5|6|7|8|%s|9|10|1|1|11|12|%d|%d|%d|1|1|0|%d|13|%d|%d|0|%s|%s|%d|A|%d|0|0|"

	// GWTRemoveServing removes a serving from the diary.
	// Parameters: header, sesnonce, serving ID, user ID.
	GWTRemoveServing = "7|0|8|https://cronometer.com/cronometer/|%s|com.cronometer.shared.rpc.CronometerService|removeServing|java.lang.String/2004016611|J|I|%s|1|2|3|4|3|5|6|7|8|%s|%s|"

	// GWTGetDayInfo returns diary entries for a day.
	// Parameters: header, sesnonce, day, month, year, user ID.
	GWTGetDayInfo = "7|0|8|https://cronometer.com/cronometer/|%s|com.cronometer.shared.rpc.CronometerService|getDayInfo|java.lang.String/2004016611|com.cronometer.shared.entries.models.Day/782579793|I|%s|1|2|3|4|3|5|6|7|8|6|%d|%d|%d|%s|"

	// GWTAddWeightBiometric adds a weight biometric. The captured payload uses metric ID 1 for weight.
	// Parameters: header, sesnonce, amount, day, month, year, user ID.
	GWTAddWeightBiometric = "7|0|9|https://cronometer.com/cronometer/|%s|com.cronometer.shared.rpc.CronometerService|addBiometric|java.lang.String/2004016611|com.cronometer.shared.biometrics.Biometric/2989635787|I|%s|com.cronometer.shared.entries.models.Day/782579793|1|2|3|4|3|5|6|7|8|6|%s|9|%d|%d|%d|0|A|0|1|0|2|0|0|0|0|0|1|0|%s|"
)
