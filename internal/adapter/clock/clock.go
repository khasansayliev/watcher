// Package clock — usecase.Clock ning haqiqiy vaqt bilan ishlovchi adapteri.
package clock

import (
	"time"
	_ "time/tzdata" // vaqt zonalari binar ichida

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

type Local struct{ loc *time.Location }

var _ usecase.Clock = Local{}

// New berilgan zonada ishlaydi (masalan "Asia/Tashkent").
func New(zone string) Local {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.FixedZone("UZT", 5*3600)
	}
	return Local{loc: loc}
}

func (c Local) Now() time.Time { return time.Now().In(c.loc) }
func (c Local) Today() string  { return c.Now().Format(domain.DateLayout) }
