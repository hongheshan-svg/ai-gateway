'use strict';
'require view';
'require form';
'require uci';
'require rpc';

return view.extend({
	load: function() {
		return uci.load('ai-gateway');
	},

	render: function() {
		var m, s, o;

		m = new form.Map('ai-gateway', _('AI Gateway - Providers'),
			_('Configure AI API providers. Enable the providers you want to intercept and provide API credentials.'));

		// Global settings
		s = m.section(form.NamedSection, 'global', 'ai-gateway', _('Global Settings'));
		s.anonymous = true;

		o = s.option(form.Flag, 'enabled', _('Enable AI Gateway'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Value, 'listen_port', _('HTTPS Listen Port'),
			_('Port for intercepting HTTPS API traffic. Default: 443'));
		o.datatype = 'port';
		o.default = '443';
		o.placeholder = '443';

		o = s.option(form.Value, 'ca_download_port', _('CA Download Port'),
			_('HTTP port for CA certificate download page. Default: 8080'));
		o.datatype = 'port';
		o.default = '8080';
		o.placeholder = '8080';

		o = s.option(form.ListValue, 'log_level', _('Log Level'));
		o.value('debug', _('Debug'));
		o.value('info', _('Info'));
		o.value('warn', _('Warning'));
		o.value('error', _('Error'));
		o.default = 'info';

		o = s.option(form.Flag, 'audit', _('Audit Logging'),
			_('Log each request with client IP, method, path, and status code.'));
		o.default = '1';

		// Anthropic Provider
		s = m.section(form.NamedSection, 'anthropic', 'provider', _('Anthropic (Claude)'));
		s.anonymous = true;

		o = s.option(form.Flag, 'enabled', _('Enable'));
		o.rmempty = false;
		o.default = '1';

		o = s.option(form.Value, 'upstream', _('Upstream URL'));
		o.default = 'https://api.anthropic.com';
		o.placeholder = 'https://api.anthropic.com';

		o = s.option(form.DynamicList, 'domains', _('Intercepted Domains'),
			_('Domains to intercept via DNS hijacking.'));
		o.default = 'api.anthropic.com';
		o.placeholder = 'api.anthropic.com';

		o = s.option(form.Value, 'api_key', _('API Key'),
			_('Anthropic API key (sk-ant-...). Used when not using OAuth.'));
		o.password = true;
		o.optional = true;

		o = s.option(form.Value, 'oauth_access_token', _('OAuth Access Token'),
			_('OAuth access token from Claude Code login. Auto-injected into requests.'));
		o.password = true;
		o.optional = true;

		o = s.option(form.Value, 'oauth_refresh_token', _('OAuth Refresh Token'),
			_('OAuth refresh token for automatic token refresh.'));
		o.password = true;
		o.optional = true;

		// OpenAI Provider
		s = m.section(form.NamedSection, 'openai', 'provider', _('OpenAI (ChatGPT)'));
		s.anonymous = true;

		o = s.option(form.Flag, 'enabled', _('Enable'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Value, 'upstream', _('Upstream URL'));
		o.default = 'https://api.openai.com';
		o.placeholder = 'https://api.openai.com';

		o = s.option(form.DynamicList, 'domains', _('Intercepted Domains'));
		o.default = 'api.openai.com';
		o.placeholder = 'api.openai.com';

		o = s.option(form.Value, 'api_key', _('API Key'),
			_('OpenAI API key (sk-...). Injected as Bearer token in Authorization header.'));
		o.password = true;
		o.optional = true;

		// Gemini Provider
		s = m.section(form.NamedSection, 'gemini', 'provider', _('Google Gemini'));
		s.anonymous = true;

		o = s.option(form.Flag, 'enabled', _('Enable'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Value, 'upstream', _('Upstream URL'));
		o.default = 'https://generativelanguage.googleapis.com';
		o.placeholder = 'https://generativelanguage.googleapis.com';

		o = s.option(form.DynamicList, 'domains', _('Intercepted Domains'));
		o.default = 'generativelanguage.googleapis.com';
		o.placeholder = 'generativelanguage.googleapis.com';

		o = s.option(form.Value, 'api_key', _('API Key'),
			_('Google Gemini API key. Injected as query parameter.'));
		o.password = true;
		o.optional = true;

		return m.render();
	}
});
