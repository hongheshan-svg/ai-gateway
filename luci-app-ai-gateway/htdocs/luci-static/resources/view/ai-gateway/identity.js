'use strict';
'require view';
'require form';
'require uci';

return view.extend({
	load: function() {
		return uci.load('ai-gateway');
	},

	render: function() {
		var m, s, o;

		m = new form.Map('ai-gateway', _('AI Gateway - Identity'),
			_('Configure the canonical device identity that all client requests will be normalized to. ' +
			  'All connected devices will appear as this single identity to AI providers.'));

		s = m.section(form.NamedSection, 'canonical', 'identity', _('Device Identity'));
		s.anonymous = true;

		o = s.option(form.Value, 'device_id', _('Device ID'),
			_('64-character hex device identifier. Set to "auto" to generate on first start.'));
		o.placeholder = 'auto';
		o.validate = function(section_id, value) {
			if (value === 'auto' || value === '') return true;
			if (/^[a-f0-9]{64}$/i.test(value)) return true;
			return _('Must be "auto" or a 64-character hex string');
		};

		o = s.option(form.Value, 'email', _('Email'),
			_('Canonical email address reported to AI providers.'));
		o.placeholder = 'user@example.com';
		o.datatype = 'minlength(3)';

		// Environment fingerprint
		s = m.section(form.NamedSection, 'canonical', 'identity', _('Environment Fingerprint'),
			_('These values replace the environment telemetry reported by Claude Code and other AI tools.'));
		s.anonymous = true;

		o = s.option(form.ListValue, 'platform', _('Platform'));
		o.value('darwin', 'macOS (darwin)');
		o.value('linux', 'Linux');
		o.value('win32', 'Windows');
		o.default = 'darwin';

		o = s.option(form.ListValue, 'arch', _('Architecture'));
		o.value('arm64', 'ARM64 (Apple Silicon)');
		o.value('x64', 'x64 (Intel/AMD)');
		o.default = 'arm64';

		o = s.option(form.Value, 'node_version', _('Node.js Version'));
		o.default = 'v24.3.0';
		o.placeholder = 'v24.3.0';

		o = s.option(form.Value, 'terminal', _('Terminal'));
		o.default = 'iTerm2.app';
		o.placeholder = 'iTerm2.app';

		o = s.option(form.Value, 'package_managers', _('Package Managers'),
			_('Comma-separated list.'));
		o.default = 'npm,pnpm';
		o.placeholder = 'npm,pnpm';

		o = s.option(form.Value, 'runtimes', _('Runtimes'));
		o.default = 'node';
		o.placeholder = 'node';

		o = s.option(form.Value, 'version', _('Claude Code Version'),
			_('Version string to report. Should match a real CC version.'));
		o.default = '2.1.81';
		o.placeholder = '2.1.81';

		o = s.option(form.Value, 'build_time', _('Build Time'));
		o.default = '2026-03-20T21:26:18Z';

		o = s.option(form.Value, 'deployment_environment', _('Deployment Environment'));
		o.default = 'unknown-darwin';

		o = s.option(form.Value, 'vcs', _('VCS'));
		o.default = 'git';

		// System Prompt Environment
		s = m.section(form.NamedSection, 'canonical', 'identity', _('System Prompt Environment'),
			_('These values replace the <env> block injected into AI system prompts. Must be consistent with environment settings above.'));
		s.anonymous = true;

		o = s.option(form.Value, 'prompt_platform', _('Prompt Platform'),
			_('Platform shown in system prompt. Should match Platform above.'));
		o.default = 'darwin';

		o = s.option(form.Value, 'prompt_shell', _('Shell'));
		o.default = 'zsh';
		o.placeholder = 'zsh';

		o = s.option(form.Value, 'prompt_os_version', _('OS Version'),
			_('OS version string (uname -sr output).'));
		o.default = 'Darwin 24.4.0';
		o.placeholder = 'Darwin 24.4.0';

		o = s.option(form.Value, 'prompt_working_dir', _('Working Directory'),
			_('Canonical working directory path prefix.'));
		o.default = '/Users/jack/projects';
		o.placeholder = '/Users/jack/projects';

		// Process Metrics
		s = m.section(form.NamedSection, 'canonical', 'identity', _('Process Metrics'),
			_('Realistic process metrics to report. Should match a real machine profile.'));
		s.anonymous = true;

		o = s.option(form.Value, 'constrained_memory', _('Physical RAM (bytes)'),
			_('e.g. 34359738368 = 32GB'));
		o.datatype = 'uinteger';
		o.default = '34359738368';

		o = s.option(form.Value, 'rss_min', _('RSS Min (bytes)'));
		o.datatype = 'uinteger';
		o.default = '300000000';

		o = s.option(form.Value, 'rss_max', _('RSS Max (bytes)'));
		o.datatype = 'uinteger';
		o.default = '500000000';

		o = s.option(form.Value, 'heap_total_min', _('Heap Total Min'));
		o.datatype = 'uinteger';
		o.default = '40000000';

		o = s.option(form.Value, 'heap_total_max', _('Heap Total Max'));
		o.datatype = 'uinteger';
		o.default = '80000000';

		o = s.option(form.Value, 'heap_used_min', _('Heap Used Min'));
		o.datatype = 'uinteger';
		o.default = '100000000';

		o = s.option(form.Value, 'heap_used_max', _('Heap Used Max'));
		o.datatype = 'uinteger';
		o.default = '200000000';

		return m.render();
	}
});
