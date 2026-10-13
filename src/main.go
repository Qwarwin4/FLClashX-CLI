package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var version = "0.2.2"

const usageRu = `FLClashX CLI — консольный VPN-клиент на ядре mihomo

Использование: flc <команда> [аргументы]

  start                        запустить VPN
  stop                         остановить VPN
  restart                      перезапустить VPN
  status                       что сейчас запущено
  server-list                  серверы активного профиля
  switch-server <сервер|№>     сменить сервер (имя, часть имени или номер)
  profile-list                 список профилей
  switch-profile <профиль|№>   сменить профиль
  servergr-name                имя группы серверов активного профиля
      --profile <имя>          у конкретного профиля
      --all-profiles           у каждого профиля
  add-profile --link <url>     добавить профиль по ссылке подписки
  add-profile --file <путь>    добавить профиль из файла
      --name <имя>             своё имя профиля
      --allow-http             разрешить ссылку без https
  update-profile [профиль]     перекачать подписку
  change-name <имя>            переименовать активный профиль
  config [--profile <имя>]     открыть yaml профиля в nano
  delete-profile <профиль>     удалить профиль (-y без вопроса)
  change-mode <режим>          rules | global | direct
  debug log [-n 200] [-f]      лог ядра
  settings                     настройки в интерактивном меню
  lang [ru|en]                 язык интерфейса
  check-updates                проверить и поставить новую версию flc
  delete                       удалить flc полностью (-y без вопроса)
  version                      версия
`

const usageEn = `FLClashX CLI — terminal VPN client on the mihomo core

Usage: flc <command> [args]

  start                        start the VPN
  stop                         stop the VPN
  restart                      restart the VPN
  status                       what's running right now
  server-list                  servers in the active profile
  switch-server <server|#>     switch server (name, part of it or number)
  profile-list                 list profiles
  switch-profile <profile|#>   switch profile
  servergr-name                server group name of the active profile
      --profile <name>         of a given profile
      --all-profiles           of every profile
  add-profile --link <url>     add a profile from a subscription link
  add-profile --file <path>    add a profile from a file
      --name <name>            custom profile name
      --allow-http             allow a non-https link
  update-profile [profile]     re-download a subscription
  change-name <name>           rename the active profile
  config [--profile <name>]    open a profile's yaml in nano
  delete-profile <profile>     delete a profile (-y to skip the prompt)
  change-mode <mode>           rules | global | direct
  debug log [-n 200] [-f]      core log
  settings                     interactive settings menu
  lang [ru|en]                 interface language
  check-updates                check for and install a new flc version
  delete                       remove flc completely (-y to skip the prompt)
  version                      version
`

func usage() string { return T(usageRu, usageEn) }

var cmds = map[string]func([]string) error{
	"start":          cmdStart,
	"stop":           cmdStop,
	"restart":        cmdRestart,
	"status":         cmdStatus,
	"server-list":    cmdServerList,
	"switch-server":  cmdSwitchServer,
	"profile-list":   cmdProfileList,
	"switch-profile": cmdSwitchProfile,
	"servergr-name":  cmdGroupName,
	"add-profile":    cmdAddProfile,
	"update-profile": cmdUpdateProfile,
	"change-name":    cmdChangeName,
	"config":         cmdConfig,
	"delete-profile": cmdDeleteProfile,
	"change-mode":    cmdChangeMode,
	"debug":          cmdDebug,
	"settings":       cmdSettings,
	"lang":           cmdLang,
	"check-updates":  cmdCheckUpdates,
	"delete":         cmdDelete,
}

func main() {
	if filepath.Base(os.Args[0]) == "flc-core" {
		coreMain()
		return
	}
	syscall.Umask(0o077)
	derr := setDirs()
	loadLang()

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage())
		os.Exit(2)
	}
	name, args := os.Args[1], os.Args[2:]
	switch name {
	case "help", "-h", "--help":
		fmt.Print(usage())
		return
	case "version", "-v", "--version":
		fmt.Println("FLClashX CLI", version)
		return
	}

	fn, ok := cmds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, T("flc: неизвестная команда %q, смотри flc help\n", "flc: unknown command %q, see flc help\n"), name)
		os.Exit(2)
	}
	if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, T("flc: не запускай от root, всё работает от обычного пользователя",
			"flc: don't run as root, everything works as a regular user"))
		os.Exit(1)
	}
	if derr == nil {
		derr = mkDirs()
	}
	if derr != nil {
		fmt.Fprintln(os.Stderr, "flc:", derr)
		os.Exit(1)
	}
	if err := fn(args); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "flc:", err)
		}
		os.Exit(1)
	}
}

func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Print(usage())
			}
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}
