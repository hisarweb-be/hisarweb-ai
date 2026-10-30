package service

// The public identity of this site: the address the gateway answers on, the
// name and domain a passkey prompt shows, the (empty) list of chat-app deep
// links and the two legal texts. None of it exists in an option table created
// from the shipped defaults, and every one of them leaks there as a code
// default that belongs to the upstream project or to a developer machine:
// "http://localhost:3000", "localhost", the upstream project name as the passkey
// name, a Chinese app list, and an empty privacy policy and user agreement so
// that /api/status reports both as disabled.
//
// Seeding follows the same rule as the rest of this file: an entry is only
// written when it is missing or empty, so an operator value always wins.

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const (
	hwServerAddressOption      = "ServerAddress"
	hwPasskeyRPIDOption        = "passkey.rp_id"
	hwPasskeyDisplayNameOption = "passkey.rp_display_name"
	hwChatAppsOption           = "Chats"
	hwUserAgreementOption      = "legal.user_agreement"
	hwPrivacyPolicyOption      = "legal.privacy_policy"
	hwDefaultServerAddress     = "https://api.hisarweb.be"
	hwDefaultPasskeyRPID       = "api.hisarweb.be"
	hwEmptyChatAppsJSON        = "[]"
	hwDefaultSiteHost          = "api.hisarweb.be"
)

// hwSeedSiteAddress sets the address the gateway answers on. The shipped
// default is an empty option, and the code default behind it is a developer
// machine; /api/status hands that address to every visitor and the mails build
// their links from it.
func hwSeedSiteAddress(stored map[string]string) error {
	if strings.TrimSpace(stored[hwServerAddressOption]) != "" {
		return nil
	}
	// An empty option leaves the code default in place, and that default is a
	// developer machine. Clear it before the write: the passkey bookkeeping in
	// UpdateOption reads the current address as the one this site answered on
	// before, and would keep "localhost" as a legacy passkey domain that
	// /api/status then lists to every visitor.
	common.OptionMapRWMutex.Lock()
	system_setting.ServerAddress = ""
	common.OptionMapRWMutex.Unlock()
	if err := model.UpdateOption(hwServerAddressOption, hwDefaultServerAddress); err != nil {
		return err
	}
	stored[hwServerAddressOption] = hwDefaultServerAddress
	return nil
}

// hwSeedPasskeyIdentity sets the name and the domain a passkey prompt shows.
// The shipped name is whatever system name the process started with, and the
// domain is derived from the server address, so both leak the local machine
// onto the public status answer. The entries belong to our own address only: on
// another address an explicit domain would be refused as unavailable, and our
// brand is not the operator's.
func hwSeedPasskeyIdentity(stored map[string]string) error {
	if !hwIsOwnSiteAddress(stored[hwServerAddressOption]) {
		return nil
	}
	if strings.TrimSpace(stored[hwPasskeyDisplayNameOption]) == "" {
		if err := model.UpdateOption(hwPasskeyDisplayNameOption, hwDefaultSystemName); err != nil {
			return err
		}
		stored[hwPasskeyDisplayNameOption] = hwDefaultSystemName
	}
	if strings.TrimSpace(stored[hwPasskeyRPIDOption]) != "" {
		return nil
	}
	if err := model.UpdateOption(hwPasskeyRPIDOption, hwDefaultPasskeyRPID); err != nil {
		return err
	}
	stored[hwPasskeyRPIDOption] = hwDefaultPasskeyRPID
	return nil
}

// hwIsOwnSiteAddress reports whether the site answers on our own address, which
// is the case on a deployment that never changed it.
func hwIsOwnSiteAddress(address string) bool {
	host := strings.TrimSpace(address)
	if after, ok := strings.CutPrefix(host, "https://"); ok {
		host = after
	} else if after, ok := strings.CutPrefix(host, "http://"); ok {
		host = after
	}
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, ":")
	return host == hwDefaultSiteHost
}

// hwSeedChatApps empties the list of chat-app deep links. The shipped list is a
// set of Chinese app links that put our key into a third-party desktop client;
// this site sells API access, not a launcher. An operator who wants the buttons
// saves a list of their own, and that value stays.
func hwSeedChatApps(stored map[string]string) error {
	if strings.TrimSpace(stored[hwChatAppsOption]) != "" {
		return nil
	}
	return model.UpdateOption(hwChatAppsOption, hwEmptyChatAppsJSON)
}

// hwSeedLegalTexts fills the privacy policy and the user agreement that the
// signup page and /api/status announce. Both start empty, which hides the two
// pages and leaves the visitor without a privacy statement; a customer in
// Türkiye needs one under KVKK. The texts stand on their own: an operator value
// is never replaced, and an operator who runs on another address still reads
// them as the statement of the service it sells.
func hwSeedLegalTexts(stored map[string]string) error {
	if strings.TrimSpace(stored[hwPrivacyPolicyOption]) == "" {
		if err := model.UpdateOption(hwPrivacyPolicyOption, hwDefaultPrivacyPolicyTR); err != nil {
			return err
		}
		stored[hwPrivacyPolicyOption] = hwDefaultPrivacyPolicyTR
	}
	if strings.TrimSpace(stored[hwUserAgreementOption]) != "" {
		return nil
	}
	if err := model.UpdateOption(hwUserAgreementOption, hwDefaultUserAgreementTR); err != nil {
		return err
	}
	stored[hwUserAgreementOption] = hwDefaultUserAgreementTR
	return nil
}

// hwDefaultPrivacyPolicyTR is the privacy statement a customer reads. It is a
// draft pending a legal review: it names the data actually processed here, the
// purpose, the retention, the transfer to the model providers and the payment
// provider, and the rights under KVKK article 11 with a working contact.
const hwDefaultPrivacyPolicyTR = `HisarWeb AI Gizlilik Politikası

Veri sorumlusu: HisarWeb AI (hisarweb.be). İletişim: hisarweb.be@gmail.com.

İşlenen veriler
- Hesap verileri: e-posta adresi, şifrenin geri döndürülemez özeti ve telefon numarasının geri döndürülemez özeti (yalnızca doğrulama amacıyla).
- Kullanım verileri: istek sayısı, seçilen model, kullanılan token sayısı, tutar ve zaman.
- Ödeme verileri: ödeme kredi kartı ile, ödeme sağlayıcıları Stripe ve Revolut üzerinden yapılır; işlem bilgileri bu sağlayıcılarda işlenir, kart bilgileri tarafımızda saklanmaz.
- Telegram verileri: telefon doğrulaması ve sorularınızın yanıtlanması için Telegram botumuz (@HisarWebAI_bot) kullanılır. Bota Telegram kullanıcı kimliğiniz, sohbet kimliğiniz, dil ayarınız, kendi isteğinizle paylaştığınız telefon numaranız ve yazdığınız mesajlar ulaşır. Telegram kullanıcı kimliğiniz hesabınıza kaydedilir; telefon numaranızın kendisi saklanmaz, yalnızca geri döndürülemez özeti tutulur. Bota serbest yazdığınız metin yanıtlanmak üzere yapay zekâ modeline iletilir. Bağlantı kodları 15 dakika geçerli ve tek kullanımlıktır. Yazışma ayrıca Telegram tarafından işlenir.
- Güvenlik verileri: oturum kayıtları, IP adresi ve cihaz bilgisi.

İşleme amacı
Hesabın açılması ve yönetimi, hizmetin sunulması, bakiyenin hesaplanması ve faturalama, dolandırıcılık ile kötüye kullanımın önlenmesi ve yasal yükümlülüklerin yerine getirilmesi.

Saklama süresi
Hesap verileri hesap açık olduğu sürece, kullanım kayıtları en çok 12 ay, faturalama kayıtları yasal saklama süresi boyunca saklanır.

Aktarım
İsteklerin işlenmesi için veriler yurt dışında bulunabilecek yapay zekâ model sağlayıcılarına, ödeme sağlayıcılarına (Stripe, Revolut) ve Telegram'a aktarılabilir. Veriler reklam amacıyla satılmaz veya kiraya verilmez.

KVKK madde 11 kapsamındaki haklarınız
Kişisel verilerinizin işlenip işlenmediğini öğrenme, işlenmişse buna ilişkin bilgi talep etme, işlenme amacını ve amacına uygun kullanılıp kullanılmadığını öğrenme, eksik veya yanlış işlenmiş olması hâlinde düzeltilmesini isteme, silinmesini veya yok edilmesini isteme, aktarıldığı üçüncü kişileri öğrenme, münhasıran otomatik sistemlerle yapılan analiz sonucu aleyhinize bir sonucun ortaya çıkmasına itiraz etme ve zararın giderilmesini talep etme hakkına sahipsiniz. Başvurunuzu hisarweb.be@gmail.com adresine iletebilirsiniz; talebiniz en geç otuz gün içinde yanıtlanır.

Çerezler
Oturumun sürdürülmesi için yalnızca gerekli çerezler kullanılır.

---

English version of this privacy policy

Controller: HisarWeb AI (hisarweb.be). Contact: hisarweb.be@gmail.com.

Data we process
- Account data: email address, an irreversible digest of your password and an irreversible digest of your phone number (for verification only).
- Usage data: number of requests, chosen model, tokens used, amount and time.
- Payment data: you pay by credit card through our payment providers Stripe and Revolut; the payment details are processed by those providers and we do not store card details.
- Telegram data: our Telegram bot (@HisarWebAI_bot) is used for phone verification and to answer your questions. The bot receives your Telegram user id, chat id, language setting, the phone number you choose to share and the messages you type. Your Telegram user id is stored on your account; your phone number itself is not stored, only an irreversible digest of it. Free text you type to the bot is sent to the AI model to be answered. Link codes are valid for 15 minutes and can be used once. The conversation is also processed by Telegram.
- Security data: session records, IP address and device information.

Purpose, retention and transfer
We process this data to open and manage your account, deliver the service, calculate your balance and invoice you, prevent fraud and abuse and meet legal duties. Account data is kept while the account is open, usage records for at most 12 months and invoicing records for the legal retention period. Data may be transferred to AI model providers, payment providers (Stripe, Revolut) and Telegram, which may be located abroad. Data is not sold or rented for advertising.

Your rights
You have the rights listed in article 11 of the Turkish KVKK. Send your request to hisarweb.be@gmail.com; it is answered within thirty days.

Cookies
Only the cookies needed to keep your session are used.`

// hwDefaultUserAgreementTR is the user agreement the signup page shows: what is
// sold, how credit is spent, what is forbidden and what is not promised. It is a
// draft pending a legal review and deliberately names no jurisdiction.
const hwDefaultUserAgreementTR = `HisarWeb AI Kullanım Koşulları

1. Hizmet
HisarWeb AI, yapay zekâ modellerine API üzerinden erişim sunar.

2. Hesap ve güvenlik
Hesap sahibi, hesabı altında yapılan tüm kullanımdan sorumludur ve API anahtarını gizli tutar. Anahtarın paylaşılmasından doğan kullanım da hesap sahibine aittir.

3. Ödeme ve kredi
Hizmet ön ödemeli kredi ile çalışır. Ödeme kredi kartı ile yapılır; ödeme sayfasını ödeme sağlayıcımız Stripe veya Revolut sunar. Bakiyeyi Türk lirası, euro veya ABD doları olarak yükleyebilirsiniz. Türk lirası fiyatlara KDV dahildir; euro ve ABD doları fiyatlarına KDV eklenmez. Kredi yalnızca gerçekleşen kullanım oranında düşülür. Satın aldığınız kredi, son ödemeli yüklemeden itibaren 30 gün geçerlidir. Ücretsiz deneme kredisi 14 gün geçerlidir ve yalnızca Telegram ile telefon doğrulamasından sonra tanımlanır. Süresi dolan kredi kullanılamaz ve bakiyeden düşülür; yeni bir yükleme süresi dolmuş krediyi geri getirmez.

4. İade
Kullanılmış kredi iade edilmez. Kullanılmamış bakiye için talep hisarweb.be@gmail.com adresine iletilebilir.

5. Yasak kullanım
Yasa dışı içerik üretmek, üçüncü kişilerin haklarını ihlal etmek, zararlı yazılım veya kimlik avı içeriği oluşturmak, hizmeti kötüye kullanmak, hız sınırlarını veya güvenlik önlemlerini aşmaya çalışmak ve deneme kredisini almak için birden çok hesap açmak yasaktır. Böyle bir kullanımda hesap askıya alınabilir veya kapatılabilir.

6. Erişilebilirlik
Hizmetin kesintisiz veya hatasız çalışacağı garanti edilmez. Bakım çalışmaları ve model sağlayıcılarından kaynaklanan kesintiler olabilir.

7. Üretilen içerik
Üretilen içeriğin kullanımından doğan sorumluluk kullanıcıya aittir. İçerik hukuki, tıbbi veya mali tavsiye yerine geçmez.

8. Değişiklik
Bu koşullar değiştirilebilir. Değişiklik bu sayfada yayımlanır ve yayımdan sonraki kullanım yeni koşullara tabidir.

9. Uygulanacak hukuk
Yürürlükteki zorunlu tüketici mevzuatından doğan haklar saklıdır.

Sorular için: hisarweb.be@gmail.com.

---

English version of sections 3 and 4 (payment, credit and refunds)

3. Payment and credit
The service runs on prepaid credit. You pay by credit card; the payment page is provided by our payment provider, Stripe or Revolut. You can top up in Turkish lira, euro or US dollars. Turkish lira prices include VAT (KDV); euro and US dollar prices do not include KDV. Credit is deducted only in proportion to actual use. Credit you buy is valid for 30 days after your last paid top-up. Free trial credit is valid for 14 days and is granted only after phone verification through Telegram. Expired credit can no longer be used and is removed from the balance; a new top-up does not bring expired credit back.

4. Refunds
Used credit is not refunded. A request for unused balance can be sent to hisarweb.be@gmail.com.`
