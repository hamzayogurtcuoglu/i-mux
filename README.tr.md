<h1 align="center">i-mux</h1>

<p align="center">
  <b>Bir yapay zekâ kodlama agent'ında kullanım limitine mi takıldınız? Sohbetin tamamıyla başka bir agent'ta devam edin.</b><br>
  Claude Code · Antigravity CLI · GitHub Copilot CLI · OpenAI Codex CLI
</p>

<p align="center">
  <img src="docs/images/limit.svg" alt="Claude limite takılıyor, i-mux Antigravity ya da Codex'te sürdürmeyi öneriyor" width="880">
</p>

i-mux, agent CLI'larınızı bir terminal çalışma alanında çalıştırır. Biri limite takıldığında ya da
siz istediğinizde tek tuşa basarsınız; aynı bölme başka bir agent ile yeniden başlar. Yeni agent
özetlenmiş bir devir belgesi alır: ilk istek, görev listesi, dokunulan dosyalar, depo durumu ve
işin kaldığı yer. Yeniden anlatmak ya da kopyalayıp yapıştırmak yok.

[tmux](https://github.com/tmux/tmux) üzerinde çalışan tek bir programdır, komutu `imux` (kendi
soketinde; sizin tmux ayarlarınıza dokunmaz). Her şey makinenizde kalır.

## Kurulum

macOS, Linux ya da WSL ile Windows'ta (bir WSL terminali açın) tek satır:

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

Betik işletim sisteminize ve işlemcinize uygun sürümü indirir, sağlama toplamını doğrular, `imux`'u
`~/.local/bin` dizinine kurar, bu dizini kabuğunuzun PATH'ine ekler (zsh, bash ya da fish) ve tmux
eksikse (3.2+; tıklanabilir agent çubuğu için 3.4+) kurmayı önerir. Ardından yeni bir terminal açın.
Seçenekler `sh -s --` sonrasına yazılır, örneğin tmux'u sormadan kurmak için `... | sh -s -- --yes`,
ya da `--version v0.1.0`, `--prefix DİZİN`, `--no-modify-path`.

Go 1.27+ ile kendiniz de derleyebilirsiniz:

```sh
go install github.com/hamzayogurtcuoglu/i-mux/cmd/imux@latest
```

Sonra en az bir agent CLI'ı kurun ve kurulumu denetleyin. Google modelleri için
[Antigravity CLI](https://antigravity.google/docs/cli/) (`agy`) kurun:
`curl -fsSL https://antigravity.google/cli/install.sh | bash`. Google kişisel hesaplara Gemini
CLI'da 2026-06-18'de hizmeti kesti; bu yüzden i-mux Gemini CLI yerine Antigravity CLI'ı destekliyor.

```sh
imux doctor
```

Kaldırmak için `~/.local/bin/imux` ve `~/.imux` silin, kabuk profilinize eklenen satırı da
(`# Added by the imux installer` ile işaretli) kaldırın.

## Kullanım

```sh
cd ~/code/projem
imux            # kurulu ilk agent'ı başlatır (ya da: imux claude)
```

| Tuş / eylem | Ne yapar |
|---|---|
| `Ctrl+\` | bölme menüsü: agent değiştir, oturumlar, yeni bölme, devir belgesi, limitler |
| `Ctrl+\` sonra `c` `a` `p` `x` | Claude / Antigravity / Copilot / Codex'te sürdür |
| alttaki çubukta bir agent'a tıklamak | o agent'ta sürdür |
| `prefix S` | projenin önceki sohbetlerinden birini seç |
| `prefix a` | bölme menüsü (terminal `Ctrl+\` tuşunu kendine ayırıyorsa) |

<table>
  <tr>
    <td width="50%"><img src="docs/images/switched.svg" alt="Antigravity oturumu i-mux'un devir belgesiyle sürdürüyor"></td>
    <td width="50%"><img src="docs/images/menu.svg" alt="i-mux bölme menüsü"></td>
  </tr>
  <tr>
    <td align="center">Antigravity, Claude'un kaldığı yerden devam ediyor</td>
    <td align="center">Bölme menüsü (<code>Ctrl+\</code>)</td>
  </tr>
</table>

Bölmede daha önce çalışmış bir agent'a geri dönmek, o agent'ın kendi oturumunu sürdürür ve ona
yalnızca aradaki gelişmeleri anlatır. Komut satırı için `imux help` (`switch`, `split`, `ls`,
`limits`, `sessions`, `handoff --print`).

Arayüz dili `LANG` / `IMUX_LANG` ya da yapılandırmadaki `locale = "tr"` ile seçilir; agent'lara
giden devir belgesi her zaman İngilizcedir.

## Bir agent limite takıldığında

Küçük bir izleyici agent ekranlarındaki limit mesajlarını fark eder ve durum çubuğunda gösterir.
Sonrasında ne olacağını `~/.imux/config.toml` içinde seçin (`imux config init` açıklamalı bir dosya
yazar):

```toml
on_limit = "ask"   # ask (varsayılan) | auto: `order` içindeki sıradaki agent'a geç | off: yalnızca göster
```

## Gizlilik

- Ağ bağlantısı, telemetri ya da API anahtarı yok. i-mux, agent'ların yerel oturum dosyalarını ve
  `git status` çıktısını okur.
- Geçiş, devir belgesini sonraki agent'ın sağlayıcısına gönderir. Gizli bilgiler elden geldiğince
  çıkarılır; `imux handoff --print` ile kontrol edin.
- Devir belgesi komut satırı argümanı olarak verilir; makinedeki diğer kullanıcılar onu `ps` ile
  görebilir. Paylaşılan makinelerde `[handoff] inject = "paste"` ayarlayın.
- Ayrıntılar: [docs/HANDOFF.md](docs/HANDOFF.md) ve [docs/AGENT-BUS.md](docs/AGENT-BUS.md).

## Belgeler (İngilizce)

[Yapılandırma](docs/CONFIG.md) · [Devir belgesi](docs/HANDOFF.md) · [Limitler](docs/LIMITS.md) ·
[Agent veri yolu](docs/AGENT-BUS.md) · [Sorun giderme](docs/TROUBLESHOOTING.md) ·
[Mimari](ARCHITECTURE.md) · [Katkı](CONTRIBUTING.md) · [English](README.md)

## Lisans

[MIT](LICENSE)
