# Початок роботи

## Встановлення

**Готові збірки для Linux.** Кожен реліз на
[Releases](https://github.com/holdemlab/sshdeck/releases) має збірки для Linux
amd64 і arm64: статичний бінарник без залежностей і пакети `.deb`, `.rpm`,
`.apk`.

```sh
# бінарник: поклади будь-куди в PATH, sudo не потрібен
tar xzf sshdeck_*_linux_amd64.tar.gz
install -m 755 sshdeck ~/.local/bin/
sha256sum -c --ignore-missing checksums.txt   # за бажанням: перевірити завантаження

# або пакет
sudo apt install ./sshdeck_*_amd64.deb        # Debian, Ubuntu
sudo dnf install ./sshdeck-*.x86_64.rpm       # Fedora, RHEL
```

На macOS, Windows та інших системах встанови через Go або збери з коду: sshdeck
там теж збирається.

**Через Go** (1.26 або новіший):

```sh
go install github.com/holdemlab/sshdeck/cmd/sshdeck@latest
```

Бінарник з'явиться в `$(go env GOPATH)/bin`.

## Збірка з коду

Потрібен Go 1.26 або новіший (цього вимагає Bubble Tea v2). Якщо встановлено
старіший Go від 1.21, команда `go` сама завантажить потрібну версію.

```sh
git clone https://github.com/holdemlab/sshdeck
cd sshdeck
make                # те саме, що go build -o bin/sshdeck ./cmd/sshdeck
./bin/sshdeck
```

Без `make` рядок з `go build` вище працює на будь-якій системі.

| Команда | Що робить |
|---|---|
| `make` / `make build` | `bin/sshdeck` для цієї машини, з версією з `git describe` |
| `make run ARGS="…"` | зібрати й запустити з прапорцями (`ARGS="-sessions /tmp/s.yaml"`) |
| `make install` | встановити в `$(go env GOPATH)/bin` |
| `make test` | тести |
| `make race` | тести з race detector, як у CI |
| `make cover` | звіт про покриття в `coverage.html` |
| `make lint` | перевірки gofmt, go vet, go mod tidy |
| `make check` | усе, що CI перевіряє в pull request |
| `make linux` | статичні бінарники для Linux amd64 і arm64 у `dist/`; їх можна просто скопіювати на будь-яку машину, sudo не потрібен |
| `make snapshot` | повна релізна збірка через GoReleaser, без публікації |
| `make next-version` | яку версію випустить наступне злиття в `main` і чому |
| `make clean` | прибрати `bin/`, `dist/` і файли покриття |

## Файли

Усе лежить у `~/.config/sshdeck/` (точніше, у `$XDG_CONFIG_HOME/sshdeck/`, якщо
ця змінна задана):

| Файл | Що в ньому | Детальніше |
|---|---|---|
| `config.yaml` | налаштування програми; створюється з коментарями при першому запуску | [settings.md](settings.md) |
| `sessions.yaml` | дерево сесій і папок | [sessions.md](sessions.md) |
| `known_hosts` | ключі серверів, яким ти довіряєш | [authentication.md](authentication.md#known_hosts) |

Окремо sshdeck веде файл стану `~/.local/state/sshdeck/state.yaml` (або
`$XDG_STATE_HOME/sshdeck/state.yaml`): які папки в дереві згорнуті. Це не
налаштування, його можна видалити будь-коли (див.
[sessions.md](sessions.md#згортання-папок)).

sshdeck не читає `~/.ssh/config` і не використовує системну програму `ssh`. Усі
налаштування живуть тільки в цих файлах.

## Прапорці командного рядка

| Прапорець | За замовчуванням | Що робить |
|---|---|---|
| `-sessions FILE` | `~/.config/sshdeck/sessions.yaml` | інший файл сесій |
| `-config FILE` | `~/.config/sshdeck/config.yaml` | інший файл налаштувань (шаблон створюється тільки для шляху за замовчуванням) |
| `-version` | | показати версію і вийти |

## Перша сесія

1. Запусти `sshdeck`. Ліворуч порожнє дерево сесій.
2. Натисни `N`, введи `prod` і натисни `enter`: з'явиться папка.
3. Натисни `n` і заповни форму:
   - Назва: `web-01`;
   - Хост: `10.0.10.11`;
   - Користувач: `deploy`;
   - Файл ключа: `~/.ssh/id_ed25519`.

   Збережи через `ctrl+s`.
4. Натисни `enter` на `web-01`, і відкриється вкладка. При першому підключенні
   sshdeck покаже відбиток ключа сервера і спитає `yes/no, так/ні`, так само як ssh
   (див. [known_hosts](authentication.md#known_hosts)).
5. Тепер ти в shell сервера. Усі клавіші йдуть на сервер, а sshdeck керується
   через prefix `ctrl+a`:
   - `ctrl+a s` — повернутися до списку сесій;
   - `ctrl+a 1`…`9` — перемкнути вкладку;
   - `ctrl+a ?` — довідка.

Що далі:
- [Сесії й папки](sessions.md): формат файлу, успадкування, редагування в UI.
- [Jump-хости](jump-hosts.md): підключення через проміжні сервери, різні ключі.
- [Тунелі](tunnels.md): прокидання закритих сервісів на локальні порти.
- [Клавіші](keys.md): повний список.
