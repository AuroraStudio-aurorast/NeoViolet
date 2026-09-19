package lyrics

import "testing"

// contractCases 是每个已注册解析器的一份代表性样例。加一个格式就必须加一条：
// TestFormatContract 会断言 AvailableParsers() 里的每个名字都在这里。
//
// embedded 的样例直接驱动 parseSYLT 而不是 embeddedParser.Parse：后者需要一个
// 真正的音频容器（io.ReadSeeker + tag.ReadFrom），内联字节串喂不进去。SYLT 正是
// embedded 唯一产出多显示行的路径（纯文本按 \n 拆成独立行，LRC 走 lrcParser），
// 容器级行为由 embedded_test.go 的真实 fixture 覆盖。
var contractCases = map[string]contractCase{
	"embedded": {
		parse: func(t *testing.T) *Data {
			t.Helper()
			d := parseSYLT(syltBody(
				syltEntry{"first\nsecond", 1000},
				syltEntry{"third", 2000},
			))
			if d == nil {
				t.Fatal("parseSYLT returned nil")
			}
			return d
		},
		expect: expectation{hasParts: true},
	},
	"eslrc": {
		parse:  viaParser("eslrc", eslrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
	"lrc": {
		parse:  viaParser("lrc", lrcContractSample),
		expect: expectation{hasParts: true, meta: true},
	},
	"lys": {
		parse: viaParser("lys", lysContractSample),
		// requireEnd 保持 false：End 由末词推出，词时缺失时退化为无界属可接受降级。
		// 样例的 End > 0 由 format_test.go 的 TestLYS_EndIsLastWordEnd 专门断言，
		// 避免这条 false 变成永远绿灯。
	},
	"qrc": {
		parse:  viaParser("qrc", qrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
	"smi": {
		parse:  viaParser("smi", smiContractSample),
		expect: expectation{hasParts: true},
	},
	"srt": {
		parse:  viaParser("srt", srtContractSample),
		expect: expectation{hasParts: true},
	},
	"ttml": {
		parse:  viaParser("ttml", ttmlAMLLSample),
		expect: expectation{requireEnd: true, hasParts: true, meta: true, looseTiling: true},
	},
	"yrc": {
		parse:  viaParser("yrc", yrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
}

const (
	lrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[00:01.00]Hello\n" +
		"[00:01.00]你好\n" +
		"[00:05.00]Bye\n"

	qrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[1000,2000]Hello(1000,500) (1500,500)world\n" +
		"[3000,2000]Bye(3000,500)\n" +
		// 平台的真实形状：词间空格写成退化的 (0,0) filler 元组，本身不带时间。
		"[5000,2000]I(5000,200) (0,0)could(5200,300) (0,0)not(5500,300)\n"

	yrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[1000,2000](1000,500,0)Hello(1500,500,0) world\n" +
		"[3000,2000](3000,500,0)Bye\n" +
		// 同一个 filler 形状，元组在文本之前。
		"[5000,2000](5000,200,0)I(0,0,0) (5200,300,0)could(0,0,0) (5500,300,0)not\n"

	// LYS 的行头是 [channel]，body 与 QRC 同形（文本在时间戳之前）。
	lysContractSample = "[0]Hello(1000,500) (1500,500)world\n" +
		"[2]Duet(3000,500)\n" +
		"[0]I(5000,200) (0,0)could(5200,300) (0,0)not(5500,300)\n"

	// 一个 SYNC 下两个 <P Class=...> 是 B 类（两个 LyricLine），<br> 是 A 类（Parts）。
	smiContractSample = "<SMI><BODY>" +
		"<SYNC Start=1000><P Class=KRCC>first<br>second" +
		"<P Class=ENCC>uno<br>dos" +
		"<SYNC Start=2000><P Class=KRCC>last" +
		"</BODY></SMI>"

	srtContractSample = "1\n" +
		"00:00:01,000 --> 00:00:03,000\n" +
		"first line\nsecond line\n\n" +
		"2\n" +
		"00:00:05,000 --> 00:00:07,000\n" +
		"bye\n"
)

// eslrcContractSample = 元数据头 + 规范逐字样例（format_test.go:200 的 testESLRC）。
// 头只加 [ti:]/[ar:] 不加 [offset:]：offset 的精确语义由 format_test.go 的单测
// 覆盖，这里保持时刻不变以便与 testESLRC 的既有断言共用一份数据。
const eslrcContractSample = "[ti:Contract]\n[ar:Tester]\n" + testESLRC
