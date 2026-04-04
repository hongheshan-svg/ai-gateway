'use strict';
'require view';
'require rpc';
'require uci';
'require fs';

function fetchCACert() {
	var port = uci.get('ai-gateway', 'global', 'ca_download_port') || '8080';
	return L.Request.get('http://' + window.location.hostname + ':' + port + '/status')
		.then(function(res) {
			try {
				return JSON.parse(res.text());
			} catch(e) {
				return null;
			}
		})
		.catch(function() {
			return null;
		});
}

return view.extend({
	load: function() {
		return Promise.all([
			uci.load('ai-gateway'),
			fetchCACert()
		]);
	},

	render: function(data) {
		var status = data[1];
		var caPort = uci.get('ai-gateway', 'global', 'ca_download_port') || '8080';
		var hostname = window.location.hostname;
		var caFingerprint = status ? status.ca_fingerprint : _('(service not running)');

		var body = E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('AI Gateway - Certificates')),

			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('CA Certificate')),
				E('table', { 'class': 'table' }, [
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td', 'width': '33%' }, E('strong', {}, _('SHA-256 Fingerprint'))),
						E('td', { 'class': 'td' }, E('code', { 'style': 'word-break:break-all;font-size:0.85em' }, caFingerprint))
					]),
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td' }, E('strong', {}, _('CA Directory'))),
						E('td', { 'class': 'td' }, E('code', {}, uci.get('ai-gateway', 'global', 'ca_dir') || '/etc/ai-gateway/ca'))
					])
				])
			]),

			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Download Certificate')),
				E('div', { 'style': 'margin: 10px 0' }, [
					E('a', {
						'href': 'http://' + hostname + ':' + caPort + '/ca.crt',
						'class': 'btn cbi-button cbi-button-action',
						'target': '_blank'
					}, _('PEM Format (.crt)')),
					' ',
					E('a', {
						'href': 'http://' + hostname + ':' + caPort + '/ca.der',
						'class': 'btn cbi-button cbi-button-action',
						'target': '_blank'
					}, _('DER Format (.der)'))
				])
			]),

			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Installation Guide')),

				E('h4', {}, 'macOS'),
				E('ol', {}, [
					E('li', {}, _('Download the PEM certificate (.crt) above')),
					E('li', {}, _('Double-click the file to open in Keychain Access')),
					E('li', {}, _('Find "AI Gateway CA" in the keychain')),
					E('li', {}, _('Double-click → Trust → "Always Trust"')),
					E('li', {}, _('Close and enter your password to confirm'))
				]),
				E('p', {}, _('Or via command line:')),
				E('pre', { 'class': 'command-line' },
					'curl -o /tmp/ai-gateway-ca.crt http://' + hostname + ':' + caPort + '/ca.crt\n' +
					'sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /tmp/ai-gateway-ca.crt'
				),

				E('h4', {}, 'Windows'),
				E('ol', {}, [
					E('li', {}, _('Download the DER certificate (.der) above')),
					E('li', {}, _('Double-click the file → "Install Certificate"')),
					E('li', {}, _('Select "Local Machine" → "Trusted Root Certification Authorities"')),
					E('li', {}, _('Complete the wizard'))
				]),
				E('p', {}, _('Or via PowerShell (admin):')),
				E('pre', { 'class': 'command-line' },
					'Invoke-WebRequest -Uri "http://' + hostname + ':' + caPort + '/ca.der" -OutFile "$env:TEMP\\ai-gateway-ca.der"\n' +
					'Import-Certificate -FilePath "$env:TEMP\\ai-gateway-ca.der" -CertStoreLocation Cert:\\LocalMachine\\Root'
				),

				E('h4', {}, 'Linux'),
				E('pre', { 'class': 'command-line' },
					'sudo curl -o /usr/local/share/ca-certificates/ai-gateway.crt http://' + hostname + ':' + caPort + '/ca.crt\n' +
					'sudo update-ca-certificates'
				),

				E('h4', {}, 'iOS / iPadOS'),
				E('ol', {}, [
					E('li', {}, E('span', {}, [
						_('Open Safari and navigate to: '),
						E('a', { 'href': 'http://' + hostname + ':' + caPort + '/ca.crt', 'target': '_blank' },
							'http://' + hostname + ':' + caPort + '/ca.crt')
					])),
					E('li', {}, _('Tap "Allow" when prompted to download the profile')),
					E('li', {}, _('Go to Settings → General → VPN & Device Management → Install')),
					E('li', {}, _('Go to Settings → General → About → Certificate Trust Settings')),
					E('li', {}, _('Enable "AI Gateway CA"'))
				]),

				E('h4', {}, 'Android'),
				E('ol', {}, [
					E('li', {}, [
						_('Download: '),
						E('a', { 'href': 'http://' + hostname + ':' + caPort + '/ca.der', 'target': '_blank' },
							'http://' + hostname + ':' + caPort + '/ca.der')
					]),
					E('li', {}, _('Settings → Security → Encryption & credentials → Install a certificate → CA certificate')),
					E('li', {}, _('Select the downloaded file'))
				])
			]),

			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Regenerate CA Certificate')),
				E('p', { 'class': 'cbi-section-descr' },
					_('Warning: Regenerating the CA certificate will require all clients to re-install the new certificate.')),
				E('button', {
					'class': 'btn cbi-button cbi-button-negative',
					'click': function() {
						if (confirm(_('Are you sure? All clients will need to reinstall the CA certificate.'))) {
							fs.exec('/bin/sh', ['-c',
								'rm -f /etc/ai-gateway/ca/ca.crt /etc/ai-gateway/ca/ca.key && ' +
								'rm -rf /tmp/ai-gateway/certs && ' +
								'/etc/init.d/ai-gateway restart'
							]).then(function() {
								window.location.reload();
							});
						}
					}
				}, _('Regenerate CA'))
			])
		]);

		return body;
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
