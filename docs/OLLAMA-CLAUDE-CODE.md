# Ollama modelleriyle Claude Code: rehber ve yapılacaklar

Tarih: 2026-11-17. Bu bir plan ve rehberdir; i-mux'ta henüz bir şey yapılmadı. İlk Ollama
denemesi (`ollama run` sohbeti, M86) bu rehberle birlikte kaldırıldı. Kodu, push edilmemiş
`feat/ollama` branch'inde duruyor (bkz. "Eski denemeden ne alınır").

---

## 1. Neden buna ihtiyacımız var?

i-mux'ın amacı, aynı projede birden çok agent'la çalışmak ve biri limite takılınca sohbeti
diğerine taşımak. Ollama'yı da bunlardan biri yapmak istiyoruz:

- **Yerel modeller** internetsiz, ücretsiz ve gizli çalışır.
- **Ollama Cloud modelleri** (GLM, Kimi, MiniMax, DeepSeek, Gemma 4…) Claude, Codex ve Copilot
  limite takıldığında aynı işe devam etmek için ucuz bir yedek olur.

## 2. İlk deneme neden işe yaramadı?

İlk denemede pane `ollama run <model>` çalıştırıyordu. Bu **agent değil, çıplak bir sohbet**:

| | Claude Code, Codex, OpenCode… | `ollama run` |
|---|---|---|
| Çalışma klasörü | Pane'in klasöründe çalışır, bunu bilir | Klasör kavramı yok |
| Dosya okuma/yazma | Araçlarıyla yapar | Yapamaz |
| Komut çalıştırma (`xcodebuild`, `git`…) | Yapar (izin isteyerek) | Yapamaz |
| Sonuç | "iPhone'a yükle" → yükler | "Yapamam" der |

Model sadece metin üretir. Bir agent, modelin etrafındaki programdır: modele araçlar tanır
("dosya oku", "komut çalıştır"), model bir aracı çağırınca program onu çalıştırır ve sonucu
modele geri verir. `ollama run` bu programı içermediği için Ollama'yı i-mux'a "sohbet" olarak
eklemek, kullanıcının beklediği şeyi hiçbir zaman veremezdi.

## 3. Çözüm: Claude Code'u Ollama'ya bağlamak

Ollama sunucusu **Anthropic Messages API**'sini taklit eder (`/v1/messages`). Claude Code ise
istekleri nereye göndereceğini `ANTHROPIC_BASE_URL` ile alır. Yani:

```
Claude Code (araçlar, klasör, izinler, oturumlar)
      │  ANTHROPIC_BASE_URL=http://localhost:11434
      ▼
Ollama sunucusu ──► yerel model (bu Mac'te)
               └──► :cloud model (Ollama'nın sunucularında)
```

Böylece model, Claude Code'un **bütün araçlarıyla**, pane'in klasöründe çalışır: dosyaları
okur, düzenler, komut çalıştırır, alt-agent'lar kullanır. Path vermeye gerek kalmaz.

**Neden özellikle Claude Code?**

- Ollama'nın en iyi desteklediği yol bu: `ollama launch claude` resmi entegrasyon ve dokümanı var.
- i-mux Claude Code'u zaten en iyi tanıyan agent: oturum dosyalarını okuyor (devir/handoff),
  `--session-id` ve `--resume` ile başlatıyor, hook'larıyla durumunu ("çalışıyor", "seni
  bekliyor") anında alıyor. Bunların hepsi Ollama'lı Claude için de bedavaya gelir.
- Alternatifler: `ollama launch` OpenCode, Codex, Copilot CLI ve başkalarını da destekliyor.
  OpenCode'u da denedik (bkz. §5); aynı donanım sorununa takıldı, ek bir avantajı yok.

## 4. Yerel modeller mi, cloud modeller mi?

### 4.1 Bu Mac'te ölçtüklerimiz (Apple M4, 16 GB)

Claude Code ve OpenCode modele ilk istekte **~36–37 bin token** gönderiyor (sistem promptu ve
araç tanımları). Yerel modelin önce bunu "okuması" gerekiyor:

| Deneme | Sonuç |
|---|---|
| Claude Code + `gemma4:12b` (12B) | 4 dk sonra zaman aşımı (Ollama 500), hiçbir iş yapılmadı |
| Claude Code + `gemma4:e4b` (8B) | Saniyede ~400 token başladı, 8–10 bin token'da bellek doldu (16 GB'ın 1,7–3,5 GB'ı boştu), swap'a düştü, 6 dk sonra zaman aşımı |
| OpenCode + `gemma4:e4b` | ~35.776 token'lık prompt, aynı şekilde takıldı |

Sonuç: **Bu makinede yerel modellerle agent çalışmaz.** Çalışabilmesi için kabaca 32 GB+
bellek ve 64k+ context gerekir. Ayrıca Ollama prompt caching desteklemediği için (§7) her
turda bu uzun prompt yeniden işlenir. Yerel seçeneği desteklemeye devam ederiz (daha güçlü
makineler için), ama bu Mac için gerçekçi yol cloud.

### 4.2 Ollama Cloud

- Modeller Ollama'nın sunucularında çalışır; indirme yok, bellek sorunu yok, hızlı.
- **Giriş:** bir Ollama hesabı ve `ollama signin` (bir kez). Giriş yapılmış yerel sunucu,
  `:cloud` ile biten modelleri kendisi cloud'a yönlendirir: Claude Code yine
  `http://localhost:11434`'e bağlanır, API anahtarı gerekmez.
- **Model adları:** `gemma4:cloud`, `kimi-k2.7-code:cloud` gibi (`<model>:cloud`). Liste:
  <https://ollama.com/search?c=cloud>. Kodlama için öne çıkanlar: `kimi-k2.7-code`, `glm-5.3`,
  `glm-5.3-flash`, `minimax-m3`. Hepsinde tools ve thinking var.
- **Ücret (2026-11-17'de <https://ollama.com/pricing>):** 31 Ağustos 2026'dan beri
  **token başına ücretli**. Eski "ücretsiz ama 5 saatlik / haftalık kota" modeli kalktı.
  - Pay as you go: $0 abonelik, kullandığın token kadar ödersin, 3 eşzamanlı istek.
  - Pro: $20/ay, $60 kullanım kredisi dahil. Max: $100/ay, $300 kredi, 10 eşzamanlı istek.
  - Fiyatlar modele göre değişir; örnek: `gpt-oss:20b` girdi $0,07 / çıktı $0,30, `kimi-k3`
    girdi $3 / çıktı $15 (milyon token başına). Bazı modellerde mesai dışı indirim var.
  - Dahil krediler aylık sıfırlanır, devretmez; %90'da e-posta gelir.
- **Gizlilik (Ollama'nın beyanı):** prompt ve cevaplar loglanmaz, eğitimde kullanılmaz.
  Sunucular ağırlıkla ABD'de, gerekirse Avrupa'ya yönlendirilir.
- **Doğrudan cloud (Ollama kurmadan):** `ANTHROPIC_BASE_URL=https://ollama.com` ve
  `ANTHROPIC_AUTH_TOKEN=$OLLAMA_API_KEY` (anahtar ollama.com'dan alınır). i-mux için yerel
  sunucu üzerinden gitmek daha basit; bu yol yalnızca Ollama kurulu olmayan makineler için.

> Eski i-mux dokümanında (ARCHITECTURE §7.8) `:cloud` modeller için "saatlik ve haftalık
> limit" yazıyordu; bu artık doğru değil. Cloud'da "limit" yerine bakiye/kredi bitmesi var.

## 5. Bugün, i-mux olmadan elle deneme

Önce bir kez:

```bash
ollama signin
```

Sonra projenin klasöründe:

```bash
ANTHROPIC_BASE_URL=http://localhost:11434 ANTHROPIC_AUTH_TOKEN=ollama ANTHROPIC_API_KEY="" claude --model kimi-k2.7-code:cloud
```

Ya da Ollama'nın kendi kısayoluyla:

```bash
ollama launch claude --model kimi-k2.7-code:cloud
```

Kontrol: Claude Code içinde `/status` bağlantının `localhost:11434` olduğunu göstermeli.
Dikkat: `~/.claude/settings.json` içindeki `env` bloğu kabuk değişkenlerini ezer; orada
`ANTHROPIC_BASE_URL` gibi bir ayar varsa bu deneme çalışmaz.

## 6. i-mux'a nasıl eklenmeli (adım adım)

Proje kuralı: önce ARCHITECTURE.md ve TASKS.md'ye yazılır ve commit'lenir, sonra kod.

### Adım 1 — Tasarım kararı: ayrı bir agent

Yeni agent: **id `ollama`, görünen ad "Ollama", ikili `claude`**. Yani Claude Code, Ollama'ya
yönlendirilmiş olarak.

Neden Claude'un bir ayarı değil de ayrı bir agent?

- **Limitler ayrı kalmalı.** Anthropic limitine takılan Claude'dan Ollama'ya geçmek asıl
  senaryo. i-mux limitleri agent id'sine göre tutuyor; aynı id olsaydı ikisi birlikte "limitli"
  görünürdü.
- Kenar panelde, "Continue this chat in"de ve menülerde ayrı bir düğme olarak görünür
  (kısayolu `m`, simgesi eski denemedeki `◆`).
- Pane kenarındaki etiket modeli gösterir: `Ollama · kimi-k2.7-code:cloud`.

### Adım 2 — Kayıt (registry) ve config

- `internal/session/session.go`: `Ollama AgentID = "ollama"`, görünen ad. Ama **oturum
  sağlayıcısı Claude'unki**: oturumlar `~/.claude/projects/` altına Claude Code formatında
  yazılır. `session.ChatOnly` işareti kaldırılır (artık gerçek oturumu var).
- `internal/agent/agent.go`: `{ID: session.Ollama, Key: "m", Bin: "claude", Vendor: "Ollama models", ReadDirFlag: "--add-dir"}`.
  Kurulum ipucu iki şey ister: Claude Code ve Ollama (`brew install ollama` / install.sh).
- `internal/config/default.toml`:

  ```toml
  [agents.ollama]
  bin         = ""                  # "" = claude
  args        = []
  new_args    = ["--model", "{model}", "--session-id", "{uuid}", "{prompt}"]
  resume_args = ["--model", "{model}", "--resume", "{id}", "{prompt}"]
  home        = ""                  # Claude'un home'u: ~/.claude
  resume      = true
  # Ollama sunucusu; doğrudan cloud için "https://ollama.com" (+ OLLAMA_API_KEY)
  base_url    = "http://localhost:11434"
  model       = ""                  # "" = pane açılınca sor
  ```

  `base_url` ve `model` yeni alanlar; `config.Agent`'a eklenir, `make generate` ile
  `docs/CONFIG.md` güncellenir.

### Adım 3 — Başlatma ortamı (asıl iş)

`internal/workspace` / `internal/launch`'ta, agent `ollama` olduğunda `launch.Spec.Env`'e:

| Değişken | Değer | Neden |
|---|---|---|
| `ANTHROPIC_BASE_URL` | `base_url` | İstekler Ollama'ya gitsin |
| `ANTHROPIC_AUTH_TOKEN` | `ollama` (cloud doğrudan: `$OLLAMA_API_KEY`) | Yerel sunucu kontrol etmez ama boş olamaz |
| `ANTHROPIC_API_KEY` | `""` | Kullanıcının gerçek Anthropic anahtarı Ollama'ya gitmesin |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | seçilen model | Claude Code arka plan işleri için `haiku` ister; Ollama'da yok, 404 olur |
| `ANTHROPIC_DEFAULT_SONNET_MODEL`, `_OPUS_MODEL`, `_FABLE_MODEL` | seçilen model | `/model sonnet` gibi takma adlar da Ollama modeline gitsin |
| `CLAUDE_CODE_SUBAGENT_MODEL` | seçilen model | Alt-agent'lar Anthropic modeli istemesin |
| `CLAUDE_CODE_MAX_CONTEXT_TOKENS` | `ollama show`'daki context length | Yoksa Claude Code tanımadığı model için 1M varsayar ve vaktinde compact etmez |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | `1` | Telemetri/güncelleme kontrolü Anthropic'e gitmesin (isteğe bağlı) |

Ayrıca kullanıcı ortamından gelen `ANTHROPIC_*` değişkenleri temizlenmeli (özellikle
`ANTHROPIC_API_KEY`), yoksa yanlış yere gider.

`--settings ~/.imux/claude-settings.json` aynen kullanılır (hook'lar ve status line), ama hook
komutu `imux __hook ollama` olmalı ki durum doğru pane/agent'a yazılsın. Bunun için
`workspace.go:503` civarındaki ayar dosyası üretimi ikinci bir dosya üretir:
`~/.imux/claude-ollama-settings.json`.

Not: Ollama'nın kendi `ollama launch claude` komutu da bunu yapıyor, ama hangi ayar dosyalarını
değiştirdiği belgelenmemiş (`--restore` seçeneği bir "profil" yazdığını düşündürüyor).
i-mux'ın kullanıcının `~/.claude` ayarlarına dokunmaması için ortam değişkeni yolu tercih
edilmeli.

### Adım 4 — Model seçici

Pane açılınca (eski denemedeki gibi, `__exec` içinde) numaralı liste:

1. `ollama list`'teki yerel modeller (sadece `ollama show`'da **tools** yeteneği olanlar),
2. önerilen cloud modeller (`kimi-k2.7-code:cloud`, `glm-5.3:cloud`, `minimax-m3:cloud`…),
   her birinin yanında "cloud · ücretli" notu,
3. Enter = en son seçilen (`~/.imux/ollama-model`), bir isim yazmak da olur.

Seçilen model `{model}` yerine geçer. `config`'te `model` doluysa soru sorulmaz.
Sunucu kapalıysa "Ollama'yı aç (`ollama serve`)" der ve tekrar sorar; cloud model seçilip
giriş yapılmamışsa "`ollama signin`" der.

Yerel model seçildiğinde bir uyarı: "Bu model ilk istekte ~37 bin token işleyecek; 32 GB'tan
az bellekte çok yavaş olabilir."

### Adım 5 — Oturumlar, devir (handoff) ve devam (resume)

Claude'un sağlayıcısı kullanıldığı için çoğu hazır:

- Ollama'dan başka bir agent'a geçerken i-mux, Claude Code oturum dosyasından sohbeti okur.
- Başka agent'tan Ollama'ya geçerken devir belgesi `{prompt}` ile verilir (Claude'daki gibi
  dosya olarak; Claude Code dosyayı kendisi okur — eski denemedeki "yapıştırma" hilesine gerek
  kalmaz).
- `--resume {id}` çalışır.
- **Karar gerekiyor:** History'de Ollama oturumları Claude oturumlarıyla aynı yerde durur.
  Ayırt etmek için i-mux, oturum id'sini başlatırken `~/.imux` altında "bu oturum Ollama'nın"
  diye kaydetmeli (pane'deki `@imux_trail` bunu zaten tutuyor) ve History'de rozetle göstermeli.

### Adım 6 — Limitler ve hatalar

- Anthropic'in limit kalıpları Ollama pane'inde **uygulanmamalı** (yanlış alarm).
- Yeni kalıplar: cloud bakiyesi/kredisi bittiğinde Ollama'nın döndürdüğü hata metni (canlı
  denemede yakalanıp testlere eklenmeli), `429 Too Many Requests` (eşzamanlı istek sınırı).
- Limits panelinde Ollama satırı: "Yerel: limit yok · Cloud: bakiye ollama.com'da".

### Adım 7 — Doctor ve kurulum

`imux doctor`: `claude` kurulu mu, Ollama sunucusu cevap veriyor mu (`/api/version`), kaç
model var, cloud için giriş yapılmış mı. Versions and updates ekranında Ollama satırı
`ollama` ikilisinin sürümünü göstermeli (agent'ın ikilisi `claude` olsa da).

### Adım 8 — Testler

- Birim: ortam değişkenlerinin doğru kurulduğu (özellikle `ANTHROPIC_API_KEY` boş), model
  seçicinin tools filtresi, `{model}` değiştirme, ayar dosyasındaki hook komutu.
- E2E: `e2e/fakeagents` altında sahte bir `claude` zaten var; bir de `ANTHROPIC_BASE_URL`'i
  kaydeden sahte bir Ollama sunucusu (küçük bir HTTP sunucusu, `/api/tags`, `/api/show`).
  Senaryo: Claude → Ollama → Claude bağlamı korur.
- Kural: gerçek `~/.claude`, `~/.ollama` okunmaz; gerçek model çalıştırılmaz.

### Adım 9 — Dokümanlar

ARCHITECTURE (yeni bir D kararı ve §7.8'in yeniden yazımı), TASKS (yeni milestone),
README ve README.tr (kurulum: `brew install ollama`, `ollama signin`, ücret notu), CHANGELOG.

## 7. Bilinen kısıtlar

Ollama'nın Anthropic uyumluluk sayfasına göre:

- **Prompt caching yok** (`cache_control` desteklenmiyor): her turda uzun sistem promptu
  yeniden işlenir. Yerelde yavaşlık, cloud'da daha fazla girdi token'ı demek.
- `tool_choice`, `/v1/messages/count_tokens`, PDF blokları, citations yok; token sayıları
  yaklaşık.
- Görseller sadece base64 olarak (Claude Code zaten öyle gönderiyor).
- Cloud'da hosted web search ve bazı gelişmiş araç kontrolleri tam desteklenmiyor.
- Claude Code tanımadığı model için "unrecognized model" uyarısı basar;
  `CLAUDE_CODE_MAX_CONTEXT_TOKENS` ile pencere düzeltilir, uyarı satırı zararsızdır.
- Model kalitesi: küçük yerel modeller uzun araç zincirlerinde (ör. iPhone'a derleyip
  yükleme) sık hata yapar. Bu tür işler için büyük cloud modelleri önerilmeli.

## 8. Eski denemeden ne alınır

`feat/ollama` branch'indeki `feat: Ollama as a sixth agent` commit'inde (push edilmedi):

- `internal/agent/ollama.go`: `OllamaModels` (`ollama list`), `ParseOllamaList`,
  `LastOllamaModel` / `SetLastOllamaModel` → model seçici için olduğu gibi kullanılabilir.
- `internal/cli/chat.go`: `__exec` içindeki numaralı model seçici (sunucu yokken tekrar
  sorma dahil) → Adım 4'ün temeli. Yapıştırma (`__chat-paste`) ve ekrandan sohbet okuma kısmı
  artık gerekmez.
- `internal/doctor/doctor.go`'daki Ollama kontrolü, sidebar simgesi ve renk (`agentFg[5]`),
  i18n metinleri, `e2e/fakeagents/fake-ollama`.
- Jobs: `ollama run --nowordwrap --hidethinking <model> <prompt>` yerine artık
  `claude -p` (Claude'un Jobs komutu) + yukarıdaki ortam değişkenleri.

## 9. Açık sorular

1. Ücretli cloud modelleri için bir harcama uyarısı/onayı gösterilsin mi?
2. History'de Ollama oturumları ayrı mı listelensin, Claude'unkilerle birlikte rozetli mi?
3. Yerel modeller listede kalsın mı, yoksa sadece bellek yetiyorsa mı gösterilsin?

## Kaynaklar

- Ollama: Claude Code entegrasyonu — <https://docs.ollama.com/integrations/claude-code>
- Ollama: Anthropic uyumluluğu — <https://docs.ollama.com/api/anthropic-compatibility>
- Ollama Cloud — <https://docs.ollama.com/cloud>, modeller <https://ollama.com/search?c=cloud>
- Ollama fiyatları — <https://ollama.com/pricing>
- Claude Code model ayarları (context penceresi, model ortam değişkenleri) —
  <https://code.claude.com/docs/en/model-config>
- Bu makinedeki ölçümler: Ollama 0.40.16, Claude Code 2.1.326, OpenCode 2.2.7, 2026-11-17.
