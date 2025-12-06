package util

import (
	"os"
	"testing"
)

func TestBadWordUtil(t *testing.T) {
	// Setup temporary bad words file
	const badWordsContent = "badword\nuglyword\n시발\n개새끼\n"
	tmpFile, err := os.CreateTemp("", "badwords_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name()) // clean up
	if _, err := tmpFile.WriteString(badWordsContent); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	util := NewBadWordUtil()
	// Initialize directly for testing without loading from Env if we want specific file
	if err := util.LoadBadWords(tmpFile.Name()); err != nil {
		t.Fatalf("Failed to load bad words: %v", err)
	}
	// Also load bytes
	if err := util.LoadBadWordsByte(tmpFile.Name()); err != nil {
		t.Fatalf("Failed to load bad words byte: %v", err)
	}

	t.Run("CheckForBadWords", func(t *testing.T) {
		tests := []struct {
			input string
			want  bool
		}{
			{"This is a clean sentence.", false},
			{"This contains a badword.", true},
			{"Mixed badword in middle.", true},
			{"시발!", true}, // Korean bad word
			{"안녕하세요", false},
			{"개새끼야", true},
		}

		for _, tt := range tests {
			got, err := util.CheckForBadWordsUsingTrie(tt.input) // Use Trie check
			if err != nil {
				t.Errorf("CheckForBadWordsUsingTrie(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("CheckForBadWordsUsingTrie(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	})

	t.Run("ReplaceBadWords", func(t *testing.T) {
		tests := []struct {
			input string
			want  string
		}{
			{"This is a clean sentence.", "This is a clean sentence."},
			{"You are a badword.", "You are a *******."},
			{"badwordbadword", "**************"}, // adjacent
			{"hello 시발 world", "hello ** world"}, // Korean replacement (2 chars, but implementation might mask bytes or runes? Let's check implementation behavior: it writes '*' per masked Rune)
			// Wait, my implementation writes '*' if MASK[byte_index] is true.
			// "시발" is 6 bytes. All 6 bytes are masked.
			// Reconstruct loop: reads rune '시' (3 bytes). mask[i] is true. Writes '*'. i+=3.
			// reads rune '발' (3 bytes). mask[i] is true. Writes '*'. i+=3.
			// So "시발" becomes "**". Correct.
			{"개새끼", "***"},
			{"some uglyword inside", "some ******** inside"},
		}

		for _, tt := range tests {
			got, err := util.ReplaceBadWords(tt.input)
			if err != nil {
				t.Errorf("ReplaceBadWords(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ReplaceBadWords(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("ReplaceBadWordsInBytes", func(t *testing.T) {
		tests := []struct {
			input string // input as string convenience
			want  string // expected output as string
		}{
			{"clean", "clean"},
			{"badword", "*******"},
			{"시발", "**"},
		}
		for _, tt := range tests {
			gotBytes, err := util.ReplaceBadWordsInBytes([]byte(tt.input))
			if err != nil {
				t.Errorf("ReplaceBadWordsInBytes error = %v", err)
			}
			got := string(gotBytes)
			if got != tt.want {
				t.Errorf("ReplaceBadWordsInBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})
}

func BenchmarkReplaceBadWords(b *testing.B) {
	// Setup
	const badWordsContent = "badword\nuglyword\n시발\n개새끼\n"
	tmpFile, err := os.CreateTemp("", "badwords_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(badWordsContent)
	tmpFile.Close()

	util := NewBadWordUtil()
	util.LoadBadWords(tmpFile.Name())

	// Use the long input text provided in the original file
	input := test10KbInput

	for b.Loop() {
		util.ReplaceBadWords(input)
	}
}

const test10KbInput = `사람을 조속하니 체인점에 있는 해결하는 붉다. 전제가 척결도 것 꼬리의 검토하나. 다양하는 때문 아침을 달리기가 의복 수 답하니. 어떤 오징어가 이윽고 외국을 이것은 지퍼를 보호하며 있는 만지자. 발전사가 왜 결국을 만들어야 통입니다 뿐 활용하기 향기롭을까. 
비교에서 잘 성실하게 산지의 매부가 할지, 먹다. 있다 자동차나 이번에 결과를 무력으로 기어들어요. 쇠약해진 마련할 5일 역사적은 환경을 남쪽을 있으며 떨어지다. 구체적까지 적은 몸체로 달이다 마는 계급께서 되다. 하다 표시하면 맞다 안으로 역사가 체제를 것 그 누구가 그 달리냐. 5,260,000원 군 있어 밤의 사장은 교수에서, 듣는 디자인이 나지 아닌가. 
행동은 닦을까 한편이 위할밖에 지하철을, 지내고 들다. 병원이 급도 시대는 나의 같이, 만하다. 관공서다 앞이 15채 삶고 예금과 줄이 말하여 것 혁명이어 그녀의 있다. 당한 그나 새롭는 보도록, 어떤 8명 진행됩니다. 그러나 팥으로 있는다 야 유지하여 거리가 궁극적도 낳읍시다 죽다. 않은 23채, 어뜩합니다 가거나, 채점하다. 
인간이게 없어야 반동에 비올라가 아지랑이다 있는 보급도 차원에 편의를 그중까지 생생하느냐. 금방 딱쥐다 일렬의 일정이 사람이 8일 소리를 오아 싸움에 있기 공개하다. 순간에 걸지 되고 민중은 말한 일단 이상하다 등보다, 바꾸다. 어 터 짓지 서로 이상하여 좀 평균대다 우리로 논함 농사꾼을 있다. 헉 일이 있다 곧장 낳고 밝히다. 장래에 있어 이루어 돈을 보다. 하나 덮다가 부녀회는 저쪽을 불어의 할 제일이는지 있다 아니. 
작품이 봉양까지 할 계속된가 같을 하고, 두다. 튀기는 보고 우주선은 거기에서 희희낙락하다. 일은 들, 장기만 즐기다, 깨끗하는 여자를 두려움에 찾아서 폭력에서 밥에게 저지른다. 담색이 그러어서 기간을 국제화를 요즘은 흘러내릴 전달되어요. 혐의에 수입에 그녀와 특수하다 것, 준비하다 계속되다. 선교회를 응 만들던 보다 치르던 앉은지 다행히 너무 연결되다. 내당과 그녀에 아서라, 담당한 분명히 화끈하여 기본을 살 우리를 것 만납니다. 선생의 이불감의 그러면, 그녀에 수레다 앞선 내려오지만 가사가 533미터 덧붙이에요. 데뷔의 번식종에 그렇으며 이맛전은 그녀로 결혼이 비록 급하자. 
복지로 불어나다 사령부가 공포의 낸다 웅크리다. 없어서 프로그램의 조사를 확실히 만도 검정고시는 있다 야물라. 서비스조차 때문 발전으로 비교하고 전도 것 어쩌는 눕습니까 법대를 버팁니다. 만들어 전철의 붙이던 잘 느끼는 품목을 머지않지 별도도 놀우. 충분하고 맞아 219조 올해로 오는 따사롭지, 붙습니다. 그런 생명의 때다, 공식을 쉽다. 하다 그로 총연봉의 한, 있다 온데간데없어진다. 보다 시간이 한 그 아는 엔고에서 숙부의 소요다 거 돌아다보지만 정벌합니다. 
봉투가 쾅 때문 잠드는 유출은 데 유지하다. 참새의 있은 것 나오아요 많습니까. 일차 일거리에서 너무 그 심해지는 여기다 지식에 새롭다. 이에서 잘 말하면 옷갓한다 남정네다 생각하에요 안기다. 지금은 아직 인간이 현상을 외갓집은 주위를 체념이, 나오다. 청한 골에 두 그지없이 더구나 운명이 만들라 슛하다. 불교를 그 어떻지만 더듬거리지 세력으로, 밤색의 탈바꿈을 기간이 1,900,000원 새롭는 치유된다. 밖이 기를 형제는 이런, 및 사퇴한다면 보내수. 첫째를 켜어야 쪽 양지꽃에 장면과 국방의 받다. 
따르다 저는 앓는다 높으라 공들인다 테이프에서, 되다. 처리다 있게 돼지에 터, 같습니다 학교부터 자기의 주인공의 수사의 밝혀지지 편해지다. 주변이는 다음이 사실 다짐에, 있자. 제도는 예절을 온수를 나라다 이런 1일, 양색시의 하얗다. 위협은 군대를 싶는다 아무는 그것도 된가. 자기도취도 시기를 검찰은 정부에 잠으로써 경보가 환경의 대하는 허덕인가. 실증적인 마찬가지가 다 건전하라 이제는 가리라 등 적용하다 아니고, 다녀올까. 
상황을 차다 뒤 논리학을 정부가 좀 속이 물리다 한다. 없다 그와 듣고, 방송이 가세요 세기에 점에 보다 힘입는다 정해지다. 좋은 영원이 신곡 모습의 무용까지 혹시 단행하여요. 년기와 전쟁은 항을 위원회도 정도를 의하다. 알려지어 글썽거립니다 42세 현관은 옮기는 터뜨리다. 지정하고 그때는 꼭대기와 안 시립으로, 하는 다니다. 
삼 주부는 주변에서 섬세하다, 가장 싶다. 갈등이 꽃답아요 만 많게 있기 병원을 정치인에서 미숙한가. 오후의 내 전이 연애나 이 개인을, 발전한다. 짊어진 자본금에서, 있으면 떠들다 길들여지다. 하룻밤을 잘할수록 제일과, 씨 매점을 물론, 있다. 성격인 불행하다 주저앉은 있다 이런 두어 인물으로 하다. 하다 피살이 계통으로 2022년 마련하게 연구를 얘기를 불러오야. 
떼어 안주는 나라가 신설에 얼굴 이 정치를 대전에 어느 걷습니다. 꾼 서양으로 말하다 가져오는, 마주 전전한다. 되다가 고친다 나머지는 실용적 잘 속도의 과일으로 후보가 못할 들어요. 잇은 원리다 것 오야붕이 원샷하다. 튀는 키우던 일보에 프로그램에서 즉 대하여도 후를 힘쓴다. 태양이 급증한 자식을 크고 수 본가 정신으로 어떤 생활과 길다니. 여전히 시장을 경로사상이 이야기의 나가고 사들인다. 하여 위원을 상대에 8번째 의미를 밭의 그러고 보듬어, 않는다. 여자를 가면서 떠나보내니, 대회를 그리고 절대적에 예능계로 관한다. 
복사와 만들고 예가 물가에 당국을 저를 재앙의 것 방문하더군. 친구와 시청자가 눈도 수상을 아니세요. 수용하게 있을 사람에 만들어서, 넓읍시다 대하다. 살과 오케이 적 교수는 안 계속하여 자신부터 있는다, 않다. 이미 신체를, 흩어질 배가 출어하기, 한 끈 절망밖에 간 집는다. 뿐 역사가 단체에 대통령이 않는다 것 길다 우주로 쪽 되다. 
매일 똑같이 비명에 꽃집으로 뿐 있더라도 틀을 있다. 받는 사명으로 있은 그 저기의, 2024년 참선도 관찰하다. 낳아요 우리말을 주걱에 수 있지 의미하다. 조건은 남게 계절은 나를 관료를 실상을 아들에 앉오. 마음을 무명으로 일부다 지위도 투입되다. 
주기 본 반영에 이때와 콩쿠르를 그대로 만만하고 2024년 듣다. 주간지에 정당하고 오게 정부가 지적으로 기념할 등 무섭어 된다 등 보다. 낼 나에 좌익을 그것의 말으로 않다. 사항과 이러할 모습과 가리킨 지하를 사실 아무래도 건너편이 하다. 없다 그런데 과학자의 얘기하다 땀의 닦아 강경이란 그 요구하여요. 그때가 문화의 위와, 아이의 즐기다 빼앗다. 만큼 있으나 부분의 예 사람을 참여에서 자색이 껴안다. 구분하여 자연조건을 바람 있다, 술 거기를 뽑다 사후다, 적절하게 세안하고 살아야지. 
신경입니다 말에서 수술이 낮은 쉽어 현실은 걸고 등잔도 합니다. 그렇어 업계 땔감이 있으라 오르라 시발ㅋㅋ. 소쿠리에서 않고 고통에서, 우리는 그, 벌써 하다. 오늘날에게 버리라 예금을 토끼 변한가. 털어놓을 최고다 일은 이름을 만화방이 학술이 주인으로 흑흑 무슨 소회에 팔까. 오빠에 그 이, 뭐는 쳐들고, 하다, 발휘하게, 양상 추가의, 밤무대를 출발한다. 부르주아적은 우리다 분급되라 길 연락이 하다. 때문 위를 무슨, 서방에서 모으다 나타나아, 리얼리즘이는 저를 거꾸로 고친지. 규정할 부르는 한 띠라, 뻗다 들려준지. 
갈까마귀도 출판은 문화는, 나가다 씨 치닫기 더욱 강하다 그도, 크림을 없읍시다. 배를 받아요 어머니를 넷을 지나는군. 노출에 볶에요 쪽 세계를 향방도, 킬로 관직을 여인 공동에 알다. 프로그램이 대학을 몇 비행기의 아래는 불편하다. 스며드쇼 많고 무대를 옛날에서 뉴스로 임신이구나 낸다 사람을 있야 어리둥절하거든. 안이 밝힌 자택에 이미지를 줄 있다. 생산자에 성격에 냉장고를 다른 보더니, 석판이고 성격에 보다. 하다 있다, 생겨나면서 나다, 곳이고 치는 두 관심 자랑하다. 한잔한다 겪는, 사람에 곧 생각한 새 버려진 중 있는 전합니다.`
