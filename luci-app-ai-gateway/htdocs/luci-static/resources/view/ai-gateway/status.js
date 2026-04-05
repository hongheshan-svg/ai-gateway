'use strict';
'require view';
'require poll';
'require rpc';
'require uci';
'require fs';

var callServiceList = rpc.declare({
	object: 'service',
	method: 'list',
	params: ['name'],
	expect: { '': {} }
});

var callGetStatus = rpc.declare({
	object: 'luci',
	method: 'getInitList',
	params: ['name'],
	expect: { '': {} }
});

function fetchGatewayStatus() {
	var port = 8080;
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
			callServiceList('ai-gateway'),
			uci.load('ai-gateway'),
			fetchGatewayStatus()
		]);
	},

	render: function(data) {
		var serviceData = data[0];
		var gatewayStatus = data[2];

		var isRunning = false;
		if (serviceData && serviceData['ai-gateway'] &&
			serviceData['ai-gateway']['instances'] &&
			serviceData['ai-gateway']['instances']['ai-gateway']) {
			isRunning = serviceData['ai-gateway']['instances']['ai-gateway']['running'];
		}

		var enabled = uci.get('ai-gateway', 'global', 'enabled') === '1';
		var listenPort = uci.get('ai-gateway', 'global', 'listen_port') || '443';
		var caPort = uci.get('ai-gateway', 'global', 'ca_download_port') || '8080';

		var statusContainer = E('div', { 'id': 'ai-gateway-status' });
		var body = E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('AI Gateway')),
			statusContainer
		]);

		var self = this;
		var renderStatus = function(svcData, gwStatus) {
			var running = false;
			if (svcData && svcData['ai-gateway'] &&
				svcData['ai-gateway']['instances'] &&
				svcData['ai-gateway']['instances']['ai-gateway']) {
				running = svcData['ai-gateway']['instances']['ai-gateway']['running'];
			}

			var content = [];

			// Service status section
			content.push(E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Service Status')),
				E('table', { 'class': 'table' }, [
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td', 'width': '33%' }, E('strong', {}, _('Enabled'))),
						E('td', { 'class': 'td' }, enabled ?
							E('span', { 'style': 'color:green' }, '✓ ' + _('Yes')) :
							E('span', { 'style': 'color:red' }, '✗ ' + _('No'))
						)
					]),
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td' }, E('strong', {}, _('Running'))),
						E('td', { 'class': 'td' }, running ?
							E('span', { 'style': 'color:green' }, '✓ ' + _('Running')) :
							E('span', { 'style': 'color:red' }, '✗ ' + _('Stopped'))
						)
					]),
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td' }, E('strong', {}, _('HTTPS Proxy Port'))),
						E('td', { 'class': 'td' }, listenPort)
					]),
					E('tr', { 'class': 'tr' }, [
						E('td', { 'class': 'td' }, E('strong', {}, _('CA Download Port'))),
						E('td', { 'class': 'td' }, E('a', {
							'href': 'http://' + window.location.hostname + ':' + caPort,
							'target': '_blank'
						}, caPort))
					])
				])
			]));

			// Provider status section
			var providers = ['anthropic', 'openai', 'gemini'];
			var providerRows = [];
			for (var i = 0; i < providers.length; i++) {
				var name = providers[i];
				var pEnabled = uci.get('ai-gateway', name, 'enabled') === '1';
				var upstream = uci.get('ai-gateway', name, 'upstream') || '-';
				var reqCount = '-';
				var errCount = '-';
				if (gwStatus && gwStatus.stats) {
					if (gwStatus.stats.providers) {
						reqCount = gwStatus.stats.providers[name] || 0;
					}
					if (gwStatus.stats.errors) {
						errCount = gwStatus.stats.errors[name] || 0;
					}
				}

				providerRows.push(E('tr', { 'class': 'tr' }, [
					E('td', { 'class': 'td' }, name.charAt(0).toUpperCase() + name.slice(1)),
					E('td', { 'class': 'td' }, pEnabled ?
						E('span', { 'style': 'color:green' }, '✓') :
						E('span', { 'style': 'color:gray' }, '✗')
					),
					E('td', { 'class': 'td' }, upstream),
					E('td', { 'class': 'td' }, String(reqCount)),
					E('td', { 'class': 'td' }, errCount > 0 ?
						E('span', { 'style': 'color:red' }, String(errCount)) :
						String(errCount)
					)
				]));
			}

			content.push(E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Provider Status')),
				E('table', { 'class': 'table' }, [
					E('tr', { 'class': 'tr cbi-section-table-titles' }, [
						E('th', { 'class': 'th' }, _('Provider')),
						E('th', { 'class': 'th' }, _('Enabled')),
						E('th', { 'class': 'th' }, _('Upstream')),
						E('th', { 'class': 'th' }, _('Requests')),
						E('th', { 'class': 'th' }, _('Errors'))
					])
				].concat(providerRows))
			]));

			// Gateway statistics
			if (gwStatus) {
				content.push(E('div', { 'class': 'cbi-section' }, [
					E('h3', {}, _('Statistics')),
					E('table', { 'class': 'table' }, [
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td', 'width': '33%' }, E('strong', {}, _('Total Requests'))),
							E('td', { 'class': 'td' }, String(gwStatus.stats ? gwStatus.stats.total_requests : 0))
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td' }, E('strong', {}, _('Active Requests'))),
							E('td', { 'class': 'td' }, String(gwStatus.stats ? gwStatus.stats.active_requests : 0))
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td' }, E('strong', {}, _('CA Fingerprint'))),
							E('td', { 'class': 'td' }, E('code', {}, gwStatus.ca_fingerprint || '-'))
						])
					])
				]));
			}

			// Update container
			while (statusContainer.firstChild)
				statusContainer.removeChild(statusContainer.firstChild);
			for (var j = 0; j < content.length; j++)
				statusContainer.appendChild(content[j]);
		};

		// Initial render
		renderStatus(serviceData, gatewayStatus);

		// CA Certificate download (static, no refresh needed)
		body.appendChild(E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, _('CA Certificate')),
			E('p', {}, _('Download and install the CA certificate on client devices to enable transparent proxying.')),
			E('div', { 'style': 'margin: 10px 0' }, [
				E('a', {
					'href': 'http://' + window.location.hostname + ':' + caPort + '/ca.crt',
					'class': 'btn cbi-button cbi-button-action',
					'target': '_blank'
				}, _('Download PEM')),
				' ',
				E('a', {
					'href': 'http://' + window.location.hostname + ':' + caPort + '/ca.der',
					'class': 'btn cbi-button cbi-button-action',
					'target': '_blank'
				}, _('Download DER')),
				' ',
				E('a', {
					'href': 'http://' + window.location.hostname + ':' + caPort,
					'class': 'btn cbi-button',
					'target': '_blank'
				}, _('Installation Guide'))
			])
		]));

		// Auto-refresh every 5 seconds
		poll.add(function() {
			return Promise.all([
				callServiceList('ai-gateway'),
				fetchGatewayStatus()
			]).then(function(res) {
				renderStatus(res[0], res[1]);
			});
		}, 5);

		return body;
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
