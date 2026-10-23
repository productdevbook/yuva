package email

import "strings"

var publicMailDomains = map[string]bool{}

func init() {
	for _, d := range strings.Fields(`
		gmail.com googlemail.com
		outlook.com outlook.de outlook.fr outlook.es outlook.it outlook.jp outlook.com.tr
		hotmail.com hotmail.co.uk hotmail.de hotmail.fr hotmail.it hotmail.es live.com live.co.uk live.de
		live.fr msn.com passport.com
		yahoo.com yahoo.co.uk yahoo.de yahoo.fr yahoo.es yahoo.it yahoo.co.jp yahoo.com.tr ymail.com
		rocketmail.com
		icloud.com me.com mac.com
		aol.com aim.com
		proton.me protonmail.com protonmail.ch pm.me
		gmx.com gmx.net gmx.de gmx.at gmx.ch web.de mail.com email.com
		t-online.de freenet.de posteo.de posteo.net mailbox.org
		yandex.com yandex.ru yandex.com.tr ya.ru
		mail.ru inbox.ru list.ru bk.ru
		zoho.com zohomail.com zohomail.eu
		fastmail.com fastmail.fm hey.com tutanota.com tutanota.de tuta.io tuta.com
		qq.com 163.com 126.com yeah.net sina.com foxmail.com
		naver.com daum.net hanmail.net
		rediffmail.com
		orange.fr wanadoo.fr free.fr laposte.net sfr.fr
		libero.it virgilio.it tiscali.it
		seznam.cz wp.pl o2.pl interia.pl onet.pl
		btinternet.com sky.com virginmedia.com
		comcast.net verizon.net att.net sbcglobal.net cox.net
	`) {
		publicMailDomains[d] = true
	}
}

// PublicMailDomain reports a mail provider's domain that many unrelated people share, where no
// one customer can prove the whole domain.
func PublicMailDomain(domain string) bool {
	return publicMailDomains[strings.ToLower(strings.TrimSuffix(domain, "."))]
}
