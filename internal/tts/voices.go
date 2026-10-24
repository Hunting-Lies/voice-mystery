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

// PlayerPool 按 playerId 稳定哈希分配（含四款音色，减少五人局重复）。
var PlayerPool = []Voice{VoiceWarmMale, VoiceSingerMale, VoiceSmartMale, VoiceBoldCuteFemale}

// HostVoice 内勤/物证播报：温暖元气男，长文通报更自然（百炼 CosyVoice）。
var HostVoice = Voice{ID: VoiceWarmMale.ID, Name: "内勤播报", Gender: "male", Remark: "龙泽·主持"}

// NarratorVoice 故事梗概与价值复盘：活泼女声（百炼 CosyVoice）。
var NarratorVoice = Voice{ID: VoiceBoldCuteFemale.ID, Name: "活泼解说", Gender: "female", Remark: "龙仙·梗概/复盘"}

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
