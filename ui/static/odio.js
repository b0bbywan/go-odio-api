function togglePlayPause(button, playerName) {
	const currentIcon = button.textContent.trim();
	const isPlaying = currentIcon === '⏸';

	const newIcon = isPlaying ? '▶' : '⏸';
	const newTitle = isPlaying ? 'Play' : 'Pause';

	button.textContent = newIcon;
	button.title = newTitle;

	fetch(`/players/${playerName}/play_pause`, {
		method: 'POST'
	}).catch(() => {
		button.textContent = currentIcon;
		button.title = isPlaying ? 'Pause' : 'Play';
		console.error('Failed to toggle play/pause');
	});
}

function updateVolume(slider, target, type) {
	const volume = slider.value / 100;

	let url;
	if (type === 'mpris') {
		url = `/players/${target}/volume`;
	} else if (type === 'audio-server') {
		url = '/audio/server/volume';
	} else if (type === 'audio-client') {
		url = `/audio/clients/${target}/volume`;
	}

	fetch(url, {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({volume: volume})
	}).catch(err => console.error('Failed to set volume:', err));
}

function toggleMute(button, target, type) {
	let url;
	if (type === 'audio-server') {
		url = '/audio/server/mute';
	} else if (type === 'audio-client') {
		url = `/audio/clients/${target}/mute`;
	}

	fetch(url, {method: 'POST'})
		.then(() => {
			button.textContent = button.textContent === '🔇' ? '🔊' : '🔇';
		})
		.catch(err => console.error('Failed to toggle mute:', err));
}
