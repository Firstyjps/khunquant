package onboard

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ANSI formatting codes.
const (
	ansiReset     = "\033[0m"
	ansiBold      = "\033[1m"
	ansiDim       = "\033[2m"
	ansiUnderline = "\033[4m"
)

const (
	lineWidth  = 72
	bodyIndent = "     "
	thinRule   = "  ──────────────────────────────────────────────────────────────────"
	thickRule  = "  ══════════════════════════════════════════════════════════════════"
	outerRule  = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
)

type legalSection struct {
	heading    string
	paragraphs []string
}

var securitySections = []legalSection{
	{
		heading: "Your Responsibility",
		paragraphs: []string{
			"You are solely responsible for installing, configuring, operating, monitoring, and securing KhunQuant and any computer, server, network, account, API key, broker credential, exchange credential, wallet, chat account, plugin, script, model provider, or third-party service connected to it.",
			"You should run KhunQuant only in an environment you trust. Keep your operating system, dependencies, browser, shell, secrets manager, and network secure. Use least-privilege API keys, disable withdrawal permissions where possible, restrict trading permissions to only what you intend to automate, and rotate credentials if compromise is suspected.",
		},
	},
	{
		heading: "Compromised Devices and Malware",
		paragraphs: []string{
			"If the device or environment running KhunQuant is compromised, hacked, infected with malware, controlled by an attacker, or configured with malicious software, an attacker may be able to read secrets, alter instructions, change trading rules, intercept messages, submit orders, access accounts, or cause other unauthorized activity within the permissions you granted.",
			"To the maximum extent permitted by applicable law, you assume all risk arising from compromise of your local environment, including loss of funds, unauthorized transactions, data disclosure, credential theft, corrupted configuration, downtime, and incorrect or manipulated AI outputs.",
		},
	},
	{
		heading: "No Security Guarantee",
		paragraphs: []string{
			"KhunQuant is provided as open-source software without any promise that it is error-free, vulnerability-free, suitable for a regulated production environment, or resistant to every attack. Security depends on your configuration, connected services, permissions, and operational practices.",
			"You should review the source code, test changes in a non-production environment, back up important data, monitor all automated actions, and independently verify any result before relying on it for financial activity.",
		},
	},
	{
		heading: "Open-Source Nature, Community Contributions, and Supply Chain Risk",
		paragraphs: []string{
			"KhunQuant is released under the MIT License and is developed with contributions from the open-source community. While maintainers make reasonable efforts to review contributions, no review process can guarantee the absence of defects, undisclosed vulnerabilities, or malicious code introduced through pull requests, dependency updates, forked repositories, or other vectors.",
			"Open-source software is inherently subject to supply chain risk. Third parties may attempt to introduce harmful code through seemingly legitimate contributions, compromised upstream dependencies, typosquatting packages, or targeted attacks on the project's distribution infrastructure. The MIT License is provided \"as is\", without warranty of any kind, express or implied, including but not limited to the warranties of merchantability, fitness for a particular purpose, and non-infringement.",
			"To the maximum extent permitted by applicable law, in no event shall the authors, maintainers, contributors, or copyright holders be liable for any claim, damages, or other liability — whether in contract, tort, or otherwise — arising from, out of, or in connection with the software or the use or other dealings in the software, including any loss of funds, unauthorized transactions, data breach, system compromise, or financial harm resulting from a vulnerability, defect, or malicious modification present in any version of the software. You use KhunQuant entirely at your own risk.",
		},
	},
}

var legalSections = []legalSection{
	{
		heading: "Software Assistant Only",
		paragraphs: []string{
			"KhunQuant provides tools for user-directed automation, notifications, portfolio tracking, and AI-assisted workflows. The software acts only according to the user's configuration, credentials, instructions, confirmations, and connected services. Any AI-generated message, plan, summary, signal, or action is an output of software running under the user's control and should not be treated as independent professional advice.",
			"KhunQuant does not solicit securities or digital asset transactions, does not recommend that any person buy, sell, hold, exchange, transfer, or withdraw any financial product, and does not determine whether any investment is suitable for any person. Users must make their own decisions and remain responsible for every instruction, approval, order, automation rule, credential permission, and transaction.",
		},
	},
	{
		heading: "Not a Regulated Intermediary",
		paragraphs: []string{
			"KhunQuant is not a digital asset exchange, broker, dealer, fund manager, advisory service, custodial wallet provider, securities broker, derivatives broker, investment consultant, or investment management service. It does not provide a market, match orders, hold client assets, custody wallets, receive money for investment, make discretionary investment decisions for clients, or act as an agent for clients in exchange for fees.",
			"Where a user connects KhunQuant to a broker, exchange, AI provider, chat platform, wallet, or other service, that service is separate from KhunQuant. Users should verify that any third-party financial service they use is properly licensed or otherwise lawful for their location and intended use.",
		},
	},
	{
		heading: "Thai SEC Regulatory Context",
		paragraphs: []string{
			"The Thai SEC identifies regulated digital asset business categories such as exchange, broker, dealer, fund manager, advisory service, and custodial wallet provider, and publishes lists of licensed or approved operators. SEC materials also warn the public to exercise caution with unlicensed investment services because such services may fall outside SEC supervision and may expose users to fraud, scams, or lack of legal protection.",
			"KhunQuant is drafted and positioned as user-operated software rather than a service that provides paid discretionary management or investment advice to the public.",
		},
	},
	{
		heading: "No Financial Advice",
		paragraphs: []string{
			"Nothing in KhunQuant, its documentation, sample prompts, examples, dashboards, alerts, model outputs, or generated messages is financial, legal, tax, accounting, investment, securities, derivatives, or digital asset advice. Market data, indicators, AI responses, summaries, and examples may be incomplete, delayed, inaccurate, or unsuitable for your circumstances.",
			"Before trading or investing, users should independently evaluate all information, consider their financial condition and risk tolerance, and consult appropriately licensed professionals where necessary.",
		},
	},
	{
		heading: "User-Directed Actions",
		paragraphs: []string{
			"Every order, transfer, withdrawal, alert, rule, schedule, or automated action configured through KhunQuant is deemed user-directed. If AI functionality prepares or executes an action, it does so only through the permissions, credentials, instructions, and connected accounts supplied by the user.",
			"Users remain solely responsible for monitoring AI behavior, disabling unintended automation, reviewing confirmations, limiting API permissions, and ensuring that all use complies with applicable law, exchange rules, broker terms, tax obligations, and platform policies.",
		},
	},
}

func printNumberedSections(sections []legalSection) {
	wrapWidth := lineWidth - len(bodyIndent)
	for i, s := range sections {
		fmt.Printf("\n%s%s%d.  %s%s\n", bodyIndent, ansiBold, i+1, s.heading, ansiReset)
		fmt.Println(thinRule)
		for _, p := range s.paragraphs {
			fmt.Println()
			for _, line := range strings.Split(wordWrap(p, wrapWidth), "\n") {
				fmt.Printf("%s%s\n", bodyIndent, line)
			}
		}
	}
}

// wordWrap wraps text at word boundaries to fit within the given width.
func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	var sb strings.Builder
	lineLen := 0
	for i, w := range words {
		if i > 0 {
			if lineLen+1+len(w) > width {
				sb.WriteByte('\n')
				lineLen = 0
			} else {
				sb.WriteByte(' ')
				lineLen++
			}
		}
		sb.WriteString(w)
		lineLen += len(w)
	}
	return sb.String()
}

// promptLegalAgreement displays the security and legal notices and asks the
// user to accept. Returns true if accepted, false if declined.
func promptLegalAgreement() bool {
	fmt.Println()
	fmt.Println(outerRule)
	fmt.Printf("  %sKhunQuant — Terms of Use%s\n", ansiBold+ansiUnderline, ansiReset)
	fmt.Printf("  %sSecurity Notice and Legal Disclaimer%s\n", ansiBold, ansiReset)
	fmt.Printf("  %sLast updated: 8 May 2026   |   Draft%s\n", ansiDim, ansiReset)
	fmt.Println(outerRule)

	fmt.Printf("\n%sPlease read the following notices carefully before proceeding.%s\n", bodyIndent, "")

	fmt.Printf("\n\n  %sPART I — SECURITY NOTICE%s\n", ansiBold, ansiReset)
	fmt.Println(thickRule)
	printNumberedSections(securitySections)

	fmt.Printf("\n\n  %sPART II — LEGAL AND REGULATORY NOTICE%s\n", ansiBold, ansiReset)
	fmt.Println(thickRule)
	printNumberedSections(legalSections)

	fmt.Println()
	fmt.Println()
	fmt.Println(outerRule)
	fmt.Printf("© 2026 KhunQuant - %s\"CryptoQuantumWave\"%s\n", ansiDim, ansiReset)
	fmt.Println()
	fmt.Printf("%sby typing Y you confirm that you have read and understood the\n", bodyIndent)
	fmt.Printf("%sabove notices and agree to use KhunQuant subject to these terms.\n", bodyIndent)
	fmt.Println(outerRule)
	fmt.Print("\n  Accept [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))

	if answer == "" || answer == "y" || answer == "yes" {
		return true
	}

	fmt.Println()
	fmt.Printf("  %sNotice:%s You must accept the legal and security terms to proceed.\n", ansiBold, ansiReset)
	fmt.Println("  Run 'khunquant onboard' again and type Y to accept.")
	fmt.Println()
	return false
}
