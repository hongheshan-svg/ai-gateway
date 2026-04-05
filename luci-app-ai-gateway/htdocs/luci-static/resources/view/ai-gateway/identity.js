'use strict';
'require view';
'require form';
'require uci';

var presets = {
	'macos_arm64': {
		label: 'macOS ARM64 (Apple Silicon)',
		platform: 'darwin', arch: 'arm64', terminal: 'iTerm2.app',
		node_version: 'v24.3.0', package_managers: 'npm,pnpm', runtimes: 'node',
		prompt_platform: 'darwin', prompt_shell: 'zsh',
		prompt_os_version: 'Darwin 24.4.0',
		prompt_working_dir: '/Users/jack/projects',
		deployment_environment: 'unknown-darwin',
		constrained_memory: '34359738368'
	},
	'ubuntu_x64': {
		label: 'Ubuntu x64',
		platform: 'linux', arch: 'x64', terminal: 'gnome-terminal',
		node_version: 'v22.12.0', package_managers: 'npm,apt', runtimes: 'node',
		prompt_platform: 'linux', prompt_shell: 'bash',
		prompt_os_version: 'Linux 6.8.0',
		prompt_working_dir: '/home/user/projects',
		deployment_environment: 'unknown-linux',
		constrained_memory: '17179869184'
	},
	'windows_x64': {
		label: 'Windows x64',
		platform: 'win32', arch: 'x64', terminal: 'Windows Terminal',
		node_version: 'v22.12.0', package_managers: 'npm', runtimes: 'node',
		prompt_platform: 'win32', prompt_shell: 'powershell',
		prompt_os_version: 'Windows NT 10.0',
		prompt_working_dir: 'C:\\Users\\user\\projects',
		deployment_environment: 'unknown-win32',
		constrained_memory: '34359738368'
	}
};

function applyPreset(presetKey) {
	var p = presets[presetKey];
	if (!p) return;
	var fields = ['platform', 'arch', 'terminal', 'node_version', 'package_managers',
		'runtimes', 'prompt_platform', 'prompt_shell', 'prompt_os_version',
		'prompt_working_dir', 'deployment_environment', 'constrained_memory'];
	for (var i = 0; i < fields.length; i++) {
		if (p[fields[i]] !== undefined) {
			uci.set('ai-gateway', 'canonical', fields[i], p[fields[i]]);
		}
	}
}

return view.extend({
	load: function() {
		return uci.load('ai-gateway');
	},

	render: function() {
		var m, s, o;

		m = new form.Map('ai-gateway', _('AI Gateway - Identity'),
			_('Configure the canonical device identity that all client requests will be normalized to. ' +
			  'All connected devices will appear as this single identity to AI providers.'));

		// Quick preset section
		s = m.section(form.NamedSection, 'canonical', 'identity', _('Quick Presets'),
			_('Apply a preset environment profile. This fills in the fields below.'));
		s.anonymous = true;

		o = s.option(form.Button, '_preset_macos', _('macOS ARM64'));
		o.inputstyle = 'action';
		o.inputtitle = _('Apply');
		o.onclick = function() {
			applyPreset('macos_arm64');
			window.location.reload();
		};

		o = s.option(form.Button, '_preset_ubuntu', _('Ubuntu x64'));
		o.inputstyle = 'action';
		o.inputtitle = _('Apply');
		o.onclick = function() {
			applyPreset('ubuntu_x64');
			window.location.reload();
		};

		o = s.option(form.Button, '_preset_windows', _('Windows x64'));
		o.inputstyle = 'action';
		o.inputtitle = _('Apply');
		o.onclick = function() {
			applyPreset('windows_x64');
			window.location.reload();
		};

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
