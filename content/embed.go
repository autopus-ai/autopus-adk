// Package content는 빌트인 컨텐츠 파일을 Go 바이너리에 임베딩한다.
// skills/, agents/, hooks/, methodology/ 하위 파일이 포함된다.
// skills/references/<skill>/ 는 해당 스킬 옆에 함께 설치되는 참조 문서이며
// 스킬 로더가 디렉터리를 건너뛰므로 별도 스킬로 등록되지 않는다.
package content

import "embed"

// FS는 임베딩된 컨텐츠 파일시스템이다.
//
//go:embed skills/*.md skills/references/*/*.md agents/*.md hooks/*.sh hooks/*.md methodology/*.yaml rules/*.md statusline.sh profiles/executor/*.md workflows/*.md workflows/*.json
var FS embed.FS
