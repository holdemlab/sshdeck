# sshdeck

English: [README.md](README.md)

Менеджер SSH-сесій для терміналу з вкладками, у дусі MobaXterm / Ásbrú.
Написаний на Go з [Bubble Tea v2](https://github.com/charmbracelet/bubbletea). Має
вбудований SSH-клієнт і емулятор терміналу, тому не потребує ні системного `ssh`,
ні tmux, і не читає `~/.ssh/config`. Збирається в один статичний бінарник.

![sshdeck: дерево сесій з кольоровими позначками, вкладка через jump-хост, Catppuccin Mocha](docs/images/screenshot.png)

## Можливості

- **Вкладки** з повноцінним терміналом: vim, htop, mc, кольори, зміна розміру,
  scrollback. Керування через prefix (`ctrl+a`, налаштовується).
- **Дерево сесій** з папками і вкладеними папками. Папки створюються,
  перейменовуються, переносяться і копіюються прямо в дереві; є пошук.
  Згортання папок (зокрема всіх разом: `H` / `L`) запам'ятовується між запусками.
- **Успадкування:** `user`, `port`, `jump`, `identity` задаються на папці й діють на
  все всередині.
- **Jump-хости і ланцюжки** (як `ProxyJump`), зі своїм ключем на кожному вузлі.
- **Авторизація:** ключі (зокрема з passphrase), ssh-agent, keyboard-interactive,
  пароль. Запити з'являються прямо у вкладці. Прийняті паролі пам'ятаються лише
  в пам'яті до виходу. `ctrl+a k` встановлює твій ключ на сервер, як
  `ssh-copy-id`.
- **Свій known_hosts** з підтвердженням нового ключа і блокуванням зміненого.
- **Тунелі** `L` / `D` (SOCKS5) / `R`: автостарт із сесії, додавання на льоту, режим
  «тільки тунелі».
- **Налаштування** редагуються з програми (`ctrl+a ,`) або в `$EDITOR`.
- **Кольорові позначки** папок і сесій (текст і заливка, `C`): середовища видно
  одразу, і в дереві, і на вкладках.
- **Кольорові схеми** [Catppuccin](https://catppuccin.com/): Latte, Frappé, Macchiato, Mocha,
  або кольори твого термінала. Фарбують і sshdeck, і програми у вкладках.
- **Дві мови:** український та англійський інтерфейс і довідка, вибір у налаштуваннях (`language: auto | uk | en`).

## Встановлення

Збірки для Linux (amd64, arm64) і пакети `.deb` / `.rpm` / `.apk` є в
[Releases](https://github.com/holdemlab/sshdeck/releases). На інших системах —
через Go 1.26+:

```sh
go install github.com/holdemlab/sshdeck/cmd/sshdeck@latest
```

З коду:

```sh
git clone https://github.com/holdemlab/sshdeck && cd sshdeck
make            # збирає bin/sshdeck; `make help` показує інші команди
./bin/sshdeck
```

Докладніше: [docs/uk/getting-started.md](docs/uk/getting-started.md).

## Швидкий старт

`N` створює папку, `n` сесію, `enter` підключає, `ctrl+a ?` показує довідку.

## Документація

Коротка довідка вбудована в програму: `ctrl+a ?` (або `?` у дереві). Це файл
[`docs/uk/help.md`](docs/uk/help.md), показаний з форматуванням, прокруткою і пошуком.

| Розділ | Про що |
|---|---|
| [Довідка](docs/uk/help.md) | усе головне на одній сторінці: клавіші, jump, ключі, тунелі, файли |
| [Початок роботи](docs/uk/getting-started.md) | збірка, файли, прапорці, перша сесія |
| [Сесії й папки](docs/uk/sessions.md) | формат `sessions.yaml`, успадкування, редагування дерева в UI |
| [Jump-хости](docs/uk/jump-hosts.md) | підключення через проміжні сервери, ланцюжки, **різні ключі на jump і на цілі** |
| [Авторизація і known_hosts](docs/uk/authentication.md) | порядок перебору ключів, passphrase, паролі, ключі серверів, keepalive |
| [Тунелі](docs/uk/tunnels.md) | `L` / `D` / `R`, панель тунелів, режим «тільки тунелі» |
| [Налаштування](docs/uk/settings.md) | усі параметри `config.yaml`, екран налаштувань, prefix і tmux |
| [Клавіші](docs/uk/keys.md) | повний список клавіш |
| [Проблеми](docs/uk/troubleshooting.md) | що означають повідомлення про помилки і що з ними робити |

Англійська документація лежить у [`docs/`](docs/).

Приклад конфігу сесій: [`examples/sessions.yaml`](examples/sessions.yaml).

## Структура коду

```
cmd/sshdeck/          точка входу, прапорці
internal/config/      sessions.yaml і config.yaml: типи, load/save, успадкування, операції з деревом
internal/sshclient/   SSH-клієнт: маршрут через jump-хости, авторизація, known_hosts, pty
internal/tunnel/      тунелі L / R / D (SOCKS5), статистика
internal/sshtest/     вбудований SSH-сервер для тестів
internal/ui/          Bubble Tea: вкладки, сайдбар, форми, тунелі, налаштування
internal/i18n/        переклади інтерфейсу (англійські ключі → українська)
docs/                 документація (en), docs/uk/ (uk); help.md і uk/help.md вбудовуються (docs.go)
```

## Що далі

1. **Multi-exec**: ввід одночасно в кілька вкладок (як cluster в Ásbrú).
2. **SFTP-браузер**: двопанельний файловий менеджер.
3. **Сплити**: дві вкладки поруч в одному вікні.
4. Миша (скрол колесом, кліки по вкладках) і пересилання ssh-agent.

Ідеї та голоси — в [issues](https://github.com/holdemlab/sshdeck/issues).

## Участь у розробці

Звіти про помилки, переклади й pull request'и вітаються. Як зібрати, перевірити й
надіслати зміни — у [CONTRIBUTING.md](CONTRIBUTING.md) (англійською); про
вразливості пиши приватно, див. [SECURITY.md](SECURITY.md).

## Ліцензія

[MIT](LICENSE) © holdemlab
