package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadLoreSection은 autopus.yaml의 lore 블록만 읽는다.
//
// 커밋 메시지 검증은 RequiredTrailers 와 StaleThresholdDays 두 값만 필요한데,
// 예전에는 config.Load 를 불러 전체 HarnessConfig.Validate 를 통과해야 했다.
// 그래서 lore 와 무관한 불변식 하나가 커밋 훅 전체를 브릭했다 - 실제로
// platforms 에 `omp` 가 추가되자 그것을 모르는 구버전 바이너리에서
// `cannot load lore config: validate config: invalid platform "omp"` 로
// 커밋이 막혔다. 설정이 앞으로 자랄 때마다 같은 일이 반복된다.
//
// 그래서 이 로더는 세 가지를 하지 않는다: 교차 필드 검증을 하지 않고,
// 디스크에 쓰지 않으며(Load 는 정규화 결과를 기록한다), lore 이외의 어떤
// 블록도 해석하지 않는다. 설정 유효성은 auto doctor 가 소유한다.
//
// 파일이 없으면 기본값을 준다. YAML 이 깨진 경우에만 실패하는데, 그때는
// 트레일러 요구를 조용히 낮추는 것보다 멈추는 편이 옳다.
func LoadLoreSection(dir string) (LoreConf, error) {
	defaults := DefaultFullConfig(filepath.Base(dir)).Lore

	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return defaults, nil
		}
		return LoreConf{}, fmt.Errorf("read config: %w", err)
	}

	// lore 블록만 담은 최소 스키마. 다른 키는 존재하든 잘못되든 무시한다.
	var envelope struct {
		Lore *LoreConf `yaml:"lore"`
	}
	expanded, _, err := expandConfigPlaceholders(data)
	if err != nil {
		return LoreConf{}, fmt.Errorf("parse config: %w", err)
	}
	if err := yaml.Unmarshal(expanded, &envelope); err != nil {
		return LoreConf{}, fmt.Errorf("parse config: %w", err)
	}
	if envelope.Lore == nil {
		return defaults, nil
	}

	// 블록이 있어도 개별 키는 생략될 수 있다. 생략된 키를 0 값으로 두면
	// 트레일러 요구가 사라지므로 기본값으로 되돌린다.
	loaded := *envelope.Lore
	if loaded.RequiredTrailers == nil {
		loaded.RequiredTrailers = defaults.RequiredTrailers
	}
	if loaded.StaleThresholdDays == 0 {
		loaded.StaleThresholdDays = defaults.StaleThresholdDays
	}
	return loaded, nil
}
