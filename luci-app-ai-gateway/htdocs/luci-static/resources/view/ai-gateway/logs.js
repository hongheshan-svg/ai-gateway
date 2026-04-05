'use strict';
'require view';
'require poll';
'require fs';

return view.extend({
	load: function() {
		return fs.exec('/usr/bin/logread', ['-e', 'ai-gateway'])
			.then(function(res) {
				return res.stdout || '';
			})
			.catch(function() {
				return '';
			});
	},

	render: function(logData) {
		var logLines = logData || _('No log entries found.');
		var logArea = E('textarea', {
			'id': 'ai-gateway-log',
			'style': 'width:100%;height:500px;font-family:monospace;font-size:12px;white-space:pre;tab-size:4;resize:vertical',
			'readonly': 'readonly',
			'wrap': 'off'
		}, logLines);

		var body = E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('AI Gateway - Logs')),
			E('div', { 'class': 'cbi-section' }, [
				E('div', { 'style': 'margin-bottom:10px' }, [
					E('button', {
						'class': 'btn cbi-button cbi-button-action',
						'click': function() {
							fs.exec('/usr/bin/logread', ['-e', 'ai-gateway'])
								.then(function(res) {
									logArea.value = res.stdout || _('No log entries found.');
									logArea.scrollTop = logArea.scrollHeight;
								});
						}
					}, _('Refresh')),
					' ',
					E('button', {
						'class': 'btn cbi-button',
						'click': function() {
							logArea.value = '';
						}
					}, _('Clear Display'))
				]),
				logArea
			])
		]);

		// Auto-scroll to bottom
		setTimeout(function() {
			logArea.scrollTop = logArea.scrollHeight;
		}, 100);

		// Auto-refresh every 5 seconds
		poll.add(function() {
			return fs.exec('/usr/bin/logread', ['-e', 'ai-gateway'])
				.then(function(res) {
					var area = document.getElementById('ai-gateway-log');
					if (area) {
						var wasAtBottom = (area.scrollHeight - area.scrollTop - area.clientHeight) < 50;
						area.value = res.stdout || '';
						if (wasAtBottom)
							area.scrollTop = area.scrollHeight;
					}
				});
		}, 5);

		return body;
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
