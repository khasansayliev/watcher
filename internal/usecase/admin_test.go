package usecase_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func (e *env) admin900() domain.User {
	return e.users.add(domain.User{TelegramChatID: "900", Name: "Admin"})
}

// newcomer — Telegram orqali yangi ro'yxatdan o'tgan foydalanuvchi (standart: ruxsatsiz).
func newcomer(t *testing.T, e *env, id string) domain.User {
	t.Helper()
	_, u, _, err := e.auth.PollLogin(context.Background(), startLogin(t, e, domain.TelegramProfile{ID: id, Name: id}))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func createFor(e *env, u domain.User) error {
	_, err := e.ws.Create(context.Background(), u.ID, usecase.CreateWatchInput{Route: domain.Route{From: "2900000", To: "2900800", Date: "2026-12-05"}})
	return err
}

func TestNewUserCannotCreateUntilAdminAllows(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	admin := e.admin900()
	u := newcomer(t, e, "555")
	if u.CanCreate {
		t.Fatal("yangi foydalanuvchi standart holatda ruxsatsiz bo'lishi kerak")
	}

	var ve *domain.ValidationError
	err := createFor(e, u)
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, "administrator ruxsati") {
		t.Fatalf("ruxsatsiz yaratish rad etilishi kerak: %v", err)
	}
	if err := e.ws.CanCreate(ctx, u.ID); !errors.As(err, &ve) {
		t.Fatal("dashboard uchun sabab qaytishi kerak")
	}

	if err := e.admin.SetCanCreate(ctx, admin, u.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := createFor(e, u); err != nil {
		t.Fatalf("ruxsatdan keyin yaratish mumkin: %v", err)
	}
	// Boshqa yangi foydalanuvchiga ruxsat tegmaydi.
	if err := createFor(e, newcomer(t, e, "666")); err == nil {
		t.Fatal("ruxsat faqat berilgan foydalanuvchiga tegishli")
	}
	// Ruxsat olinsa, yangi yaratib bo'lmaydi, mavjud kuzatuv esa qoladi.
	_ = e.admin.SetCanCreate(ctx, admin, u.ID, false)
	if err := createFor(e, u); err == nil {
		t.Fatal("ruxsat olingach yaratib bo'lmaydi")
	}
	if list, _ := e.ws.List(ctx, u.ID); len(list) != 1 {
		t.Fatal("mavjud kuzatuv saqlanishi kerak")
	}
}

func TestCreationModes(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	admin := e.admin900()
	plain := newcomer(t, e, "555")
	allowed := newcomer(t, e, "666")
	_ = e.admin.SetCanCreate(ctx, admin, allowed.ID, true)

	_ = e.admin.SetCreationMode(ctx, admin, domain.CreationAll)
	if err := createFor(e, plain); err != nil {
		t.Fatalf("'hamma' rejimida ruxsatsiz ham yarata oladi: %v", err)
	}

	_ = e.admin.SetCreationMode(ctx, admin, domain.CreationNone)
	if err := createFor(e, allowed); err == nil || !strings.Contains(err.Error(), "to'xtatilgan") {
		t.Fatalf("'hech kim' rejimida ruxsat berilgan ham yarata olmaydi: %v", err)
	}
	if err := createFor(e, admin); err != nil {
		t.Fatalf("admin har doim yarata oladi: %v", err)
	}

	_ = e.admin.SetCreationMode(ctx, admin, domain.CreationApproved)
	if err := createFor(e, allowed); err != nil {
		t.Fatalf("'ruxsat berilganlar' rejimida allowed yarata oladi: %v", err)
	}
	if err := createFor(e, newcomer(t, e, "777")); err == nil {
		t.Fatal("ruxsatsiz yarata olmasligi kerak")
	}

	var ve *domain.ValidationError
	if err := e.admin.SetCreationMode(ctx, admin, "nomalum"); !errors.As(err, &ve) {
		t.Fatalf("noma'lum rejim rad etilishi kerak: %v", err)
	}
}

func TestAdminActionsRequireAdmin(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	user := e.user("A")
	w := mustCreate(t, e, user, "2900000", "2900700", "2026-12-05")

	checks := map[string]error{
		"Users":            func() error { _, err := e.admin.Users(ctx, user, 1); return err }(),
		"SetCanCreate":     e.admin.SetCanCreate(ctx, user, user.ID, true),
		"SetUserDisabled":  e.admin.SetUserDisabled(ctx, user, user.ID, true),
		"Watches":          func() error { _, err := e.admin.Watches(ctx, user, 0, 1); return err }(),
		"SetWatchDisabled": e.admin.SetWatchDisabled(ctx, user, w.ID, true),
		"SetCreationMode":  e.admin.SetCreationMode(ctx, user, domain.CreationAll),
	}
	for name, err := range checks {
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s: oddiy foydalanuvchiga ruxsat bo'lmasligi kerak: %v", name, err)
		}
	}
	if got, _ := e.watches.Get(ctx, w.ID); got.AdminDisabled {
		t.Fatal("rad etilgan amal ta'sir qilmasligi kerak")
	}
}

func TestDisabledUserCannotLoginAndWatchesStop(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	admin := e.admin900()
	u := newcomer(t, e, "555")
	_ = e.admin.SetCanCreate(ctx, admin, u.ID, true)
	w := mustCreate(t, e, u, "2900000", "2900800", "2026-12-05")
	token, _, _, _ := e.auth.PollLogin(ctx, startLogin(t, e, domain.TelegramProfile{ID: "555"}))

	if err := e.admin.SetUserDisabled(ctx, admin, admin.ID, true); err == nil {
		t.Fatal("admin o'zini o'chira olmasligi kerak")
	}
	if err := e.admin.SetUserDisabled(ctx, admin, u.ID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("o'chirilgan foydalanuvchi sessiyasi tugashi kerak: %v", err)
	}
	code, _, _ := e.auth.BeginLogin(ctx)
	before := e.notif.count("555")
	if err := e.auth.ConfirmLogin(ctx, code, domain.TelegramProfile{ID: "555"}); err != nil {
		t.Fatal(err)
	}
	if e.notif.count("555") != before+1 {
		t.Fatal("botda sababi aytilishi kerak")
	}
	if _, _, _, err := e.auth.PollLogin(ctx, code); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("o'chirilgan foydalanuvchi sessiya ololmasligi kerak: %v", err)
	}

	calls := e.search.calls
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != calls {
		t.Fatal("o'chirilgan foydalanuvchi kuzatuvlari tekshirilmasligi kerak")
	}
	if e.notif.count("555") != before+1 {
		t.Fatal("o'chirilgan foydalanuvchiga xabar ketmasligi kerak")
	}

	// Qayta yoqilsa — hammasi tiklanadi.
	_ = e.admin.SetUserDisabled(ctx, admin, u.ID, false)
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != calls+1 {
		t.Fatal("qayta yoqilgach kuzatuv tekshirilishi kerak")
	}
	if _, _, _, err := e.auth.PollLogin(ctx, startLogin(t, e, domain.TelegramProfile{ID: "555"})); err != nil {
		t.Fatalf("qayta yoqilgach kira oladi: %v", err)
	}
	_ = w
}

func TestAdminDisabledWatch(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	admin := e.admin900()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900800", "2026-12-05")

	if err := e.admin.SetWatchDisabled(ctx, admin, w.ID, true); err != nil {
		t.Fatal(err)
	}
	calls := e.search.calls
	e.monitor.RunCycle(ctx, 0)
	e.monitor.RunCycle(ctx, w.ID)
	if e.search.calls != calls {
		t.Fatal("admin to'xtatgan kuzatuv tekshirilmasligi kerak")
	}

	var ve *domain.ValidationError
	if _, err := e.ws.Toggle(ctx, u.ID, w.ID); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "administrator") {
		t.Fatalf("foydalanuvchi admin to'xtatganini o'zi yoqa olmaydi: %v", err)
	}
	if err := e.ws.RequestCheck(ctx, u.ID, w.ID); !errors.As(err, &ve) {
		t.Fatal("qo'lda tekshiruv ham rad etilishi kerak")
	}
	// Foydalanuvchi uni o'chira oladi (o'z kuzatuvi).
	if got, _ := e.watches.Get(ctx, w.ID); !got.AdminDisabled || got.Active != true {
		t.Fatalf("holat: %+v", got)
	}

	_ = e.admin.SetWatchDisabled(ctx, admin, w.ID, false)
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != calls+1 {
		t.Fatal("qayta yoqilgach tekshirilishi kerak")
	}
	if _, err := e.ws.Toggle(ctx, u.ID, w.ID); err != nil {
		t.Fatalf("admin yoqqach foydalanuvchi boshqara oladi: %v", err)
	}
}

func TestAdminListsUsersAndWatchesWithPagination(t *testing.T) {
	e := newEnv(100, 0)
	ctx := context.Background()
	admin := e.admin900()
	var first domain.User
	for i := 0; i < usecase.AdminPerPage+3; i++ {
		u := e.user("U" + strconv.Itoa(i))
		if i == 0 {
			first = u
		}
		mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	}

	p1, err := e.admin.Users(ctx, admin, 1)
	if err != nil {
		t.Fatal(err)
	}
	// admin ham ro'yxatda (e.admin900 qo'shgan) — jami AdminPerPage+4
	if p1.Total != usecase.AdminPerPage+4 || p1.Pages != 2 || len(p1.Users) != usecase.AdminPerPage {
		t.Fatalf("foydalanuvchilar 1-sahifa: %+v", p1.Pager)
	}
	if p1.Settings.CreationMode != domain.CreationApproved {
		t.Fatal("standart rejim: faqat ruxsat berilganlar")
	}
	if p2, _ := e.admin.Users(ctx, admin, 2); len(p2.Users) != 4 {
		t.Fatalf("2-sahifa: %d", len(p2.Users))
	}
	if far, _ := e.admin.Users(ctx, admin, 99); far.Page != 2 {
		t.Fatalf("chegaradan tashqari sahifa: %d", far.Page)
	}

	all, err := e.admin.Watches(ctx, admin, 0, 1)
	if err != nil || all.Total != usecase.AdminPerPage+3 || all.Owner != nil {
		t.Fatalf("barcha kuzatuvlar: %+v %v", all.Pager, err)
	}
	if all.Watches[0].Owner.TelegramChatID == "" {
		t.Fatal("egasi ma'lumoti bo'lishi kerak")
	}
	one, err := e.admin.Watches(ctx, admin, first.ID, 1)
	if err != nil || one.Total != 1 || one.Owner == nil || one.Owner.ID != first.ID {
		t.Fatalf("bitta foydalanuvchi filtri: %+v %v", one.Pager, err)
	}
	if _, err := e.admin.Watches(ctx, admin, 9999, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("noma'lum foydalanuvchi: %v", err)
	}
}
