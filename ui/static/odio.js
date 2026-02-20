function togglePlayPause(button, playerName) {
	const currentIcon = button.textContent.trim();
	const isPlaying = currentIcon === '⏸';

	const newIcon = isPlaying ? '▶' : '⏸';
	const newTitle = isPlaying ? 'Play' : 'Pause';

	button.textContent = newIcon;
	button.title = newTitle;

	// Update data-playing immediately so the seeker stops/starts without
	// waiting for the next HTMX poll.
	const slider = document.querySelector(`.seek-slider[data-player="${CSS.escape(playerName)}"]`);
	if (slider) {
		const val = isPlaying ? 'false' : 'true';
		slider.dataset.playing = val;
		const span = slider.previousElementSibling;
		if (span) span.dataset.playing = val;
	}

	fetch(`/players/${playerName}/play_pause`, {
		method: 'POST'
	}).catch(() => {
		button.textContent = currentIcon;
		button.title = isPlaying ? 'Pause' : 'Play';
		if (slider) {
			const val = isPlaying ? 'true' : 'false';
			slider.dataset.playing = val;
			const span = slider.previousElementSibling;
			if (span) span.dataset.playing = val;
		}
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

// ── Position seeker ──────────────────────────────────────────────────────────

function fmtMicros(us) {
	const total = Math.floor(us / 1_000_000);
	const m = Math.floor(total / 60);
	const s = total % 60;
	return `${m}:${s.toString().padStart(2, '0')}`;
}

// Returns the estimated current position in microseconds, interpolated from
// the cache timestamp embedded in the element's data attributes.
function currentPos(el) {
	const base = parseInt(el.dataset.position);
	if (el.dataset.playing !== 'true' || !el.dataset.cacheUpdatedAt) return base;
	const elapsed = Date.now() - new Date(el.dataset.cacheUpdatedAt).getTime(); // ms
	return base + elapsed * 1000 * (parseFloat(el.dataset.rate) || 1.0); // → µs
}

// While dragging: update the position label without making any API call.
function onSeekerInput(slider) {
	slider.previousElementSibling.textContent = fmtMicros(parseInt(slider.value));
}

// On release: send absolute target position; the backend resolves the track ID
// and computes the relative offset if needed.
function onSeekerChange(slider) {
	const position = parseInt(slider.value);
	// Optimistically update data attributes so updatePositions interpolates
	// from the new position instead of snapping back to the old cached one.
	slider.dataset.position = position;
	slider.dataset.cacheUpdatedAt = new Date().toISOString();
	fetch(`/players/${slider.dataset.player}/position`, {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({position: position})
	}).catch(err => console.error('Seek failed:', err));
}

// Tick all seekers every 500 ms. Queries the live DOM each time so HTMX
// section replacements are handled automatically without re-initialisation.
function updatePositions() {
	document.querySelectorAll('.seek-slider').forEach(slider => {
		const pos = Math.min(currentPos(slider), parseInt(slider.max));
		slider.value = pos;
		slider.previousElementSibling.textContent = fmtMicros(pos);
	});
}
document.addEventListener('DOMContentLoaded', () => setInterval(updatePositions, 500));
document.addEventListener('htmx:afterSwap', updatePositions);

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
