package webui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
)

// So'rovlar orasidagi o'rtacha pauza (2–4 s) tufayli saytga soatiga taxminan shuncha so'rov sig'adi.
const practicalRequestsPerHour = 1200

type ruleRowVM struct {
	Index     int
	Days      string // bo'sh — yangi qator
	Value     string
	Unit      string // s | m | h
	IsDefault bool
	IsNew     bool
}

type tierVM struct {
	Label, Interval string
	Routes, Watches int
	RequestsPerHour string
}

type adminVM struct {
	layoutVM
	Rows         []ruleRowVM
	Count        int
	Tiers        []tierVM
	TotalPerHour string
	OverCapacity bool
	Capacity     int
	UpdatedInfo  string
	MinInterval  string
}

func (s *Server) adminPolling(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	stats, err := s.d.Polling.Stats(r.Context(), u)
	if err != nil {
		if r.Method == http.MethodGet && !u.IsAdmin(s.d.AdminIDs) {
			http.NotFound(w, r)
			return
		}
		s.fail(w, r, "/", err)
		return
	}
	policy, err := s.d.Polling.Policy(r.Context())
	if err != nil {
		s.fail(w, r, "/", err)
		return
	}

	vm := adminVM{layoutVM: s.adminLayout("Admin · Tekshiruv oraliqlari", "polling", r, u), Capacity: practicalRequestsPerHour,
		MinInterval: textfmt.Interval(domain.MinCheckInterval)}
	for _, rule := range policy.Rules {
		val, unit := splitInterval(rule.Interval)
		row := ruleRowVM{Value: val, Unit: unit, IsDefault: rule.IsDefault()}
		if !rule.IsDefault() {
			row.Days = strconv.Itoa(rule.MaxDaysLeft)
		}
		vm.Rows = append(vm.Rows, row)
	}
	for i := 0; i < 2; i++ {
		vm.Rows = append(vm.Rows, ruleRowVM{Unit: "m", IsNew: true})
	}
	for i := range vm.Rows {
		vm.Rows[i].Index = i
	}
	vm.Count = len(vm.Rows)

	var total float64
	for _, t := range stats {
		total += t.RequestsPerHour
		label, interval := t.Rule.Label(), "har "+textfmt.Interval(t.Rule.Interval)
		if t.Custom {
			label, interval = "Admin belgilagan oraliqlar", "alohida"
		}
		vm.Tiers = append(vm.Tiers, tierVM{
			Label: label, Interval: interval,
			Routes: t.Routes, Watches: t.Watches, RequestsPerHour: perHour(t.RequestsPerHour),
		})
	}
	vm.TotalPerHour = perHour(total)
	vm.OverCapacity = total > practicalRequestsPerHour
	if !policy.UpdatedAt.IsZero() && policy.UpdatedBy != "" {
		vm.UpdatedInfo = fmt.Sprintf("Oxirgi o'zgarish: %s, %s", policy.UpdatedBy,
			policy.UpdatedAt.In(s.d.Clock.Now().Location()).Format("02.01.2006 15:04"))
	}
	s.render(w, "admin.html", vm)
}

func (s *Server) adminPollingSave(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if !u.IsAdmin(s.d.AdminIDs) {
		http.NotFound(w, r)
		return
	}
	n, _ := strconv.Atoi(r.FormValue("n"))
	if n < 0 || n > 50 {
		s.back(w, r, "/admin/polling", "Noto'g'ri forma", "err")
		return
	}
	var rules []domain.PollingRule
	for i := 0; i < n; i++ {
		k := strconv.Itoa(i)
		days := strings.TrimSpace(r.FormValue("d" + k))
		val := strings.TrimSpace(r.FormValue("v" + k))
		if r.FormValue("x"+k) != "" || (days == "" && val == "") {
			continue // o'chirilgan yoki bo'sh qator
		}
		d, err1 := strconv.Atoi(days)
		v, err2 := strconv.Atoi(val)
		if err1 != nil || err2 != nil || v <= 0 {
			s.back(w, r, "/admin/polling", fmt.Sprintf("%d-qator: kun va oraliq butun son bo'lishi kerak", i+1), "err")
			return
		}
		rules = append(rules, domain.PollingRule{MaxDaysLeft: d, Interval: time.Duration(v) * unitOf(r.FormValue("u"+k))})
	}
	if _, err := s.d.Polling.Update(r.Context(), u, rules); err != nil {
		s.fail(w, r, "/admin/polling", err)
		return
	}
	s.back(w, r, "/admin/polling", "Jadval saqlandi — keyingi tekshiruv siklidan kuchga kiradi", "ok")
}

func unitOf(u string) time.Duration {
	switch u {
	case "h":
		return time.Hour
	case "m":
		return time.Minute
	default:
		return time.Second
	}
}

func splitInterval(d time.Duration) (string, string) {
	switch {
	case d%time.Hour == 0:
		return strconv.Itoa(int(d / time.Hour)), "h"
	case d%time.Minute == 0:
		return strconv.Itoa(int(d / time.Minute)), "m"
	default:
		return strconv.Itoa(int(d / time.Second)), "s"
	}
}

func perHour(v float64) string {
	if v > 0 && v < 1 {
		return fmt.Sprintf("%.1f", v)
	}
	return textfmt.Sum(math.Round(v))
}
