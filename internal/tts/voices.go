package tts

// 与灵矽 xrobotd 种子音色对齐（Aliyun CosyVoice / TTS_AliyunDoubleStreamTTS）。
// 来源：main/manager-api/.../202507100000.sql

type Voice struct {
	ID     string `json:"id"`     // CosyVoice voice 编码
	Name   string `json:"name"`   // 展示名
	Gender string `json:"gender"` // male | female
	Remark string `json:"remark"` // 官方备注名
}

// 男：温暖元气 / 实力歌手 / 智慧青年；女：豪放可爱（主持默认）
var (
	VoiceWarmMale     = Voice{ID: "longze_v2", Name: "温暖元气男", Gender: "male", Remark: "龙泽"}
	VoiceSingerMale   = Voice{ID: "kabuleshen_v2", Name: "实力歌手男", Gender: "male", Remark: "龙深"}
	VoiceSmartMale    = Voice{ID: "longcheng_v2", Name: "智慧青年男", Gender: "male", Remark: "龙橙"}
	VoiceBoldCuteFemale = Voice{ID: "longxian_v2", Name: "豪放可爱女", Gender: "female", Remark: "龙仙"}
)

// PlayerPool 按入座顺序轮换给玩家（不含主持）。
var PlayerPool = []Voice{VoiceWarmMale, VoiceSingerMale, VoiceSmartMale}

// HostVoice 主持播报默认音色。
var HostVoice = VoiceBoldCuteFemale

func Catalog() []Voice {
	return []Voice{VoiceWarmMale, VoiceSingerMale, VoiceSmartMale, VoiceBoldCuteFemale}
}

func Lookup(id string) (Voice, bool) {
	for _, v := range Catalog() {
		if v.ID == id {
			return v, true
		}
	}
	return Voice{}, false
}
