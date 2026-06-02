// ── Toasts ──────────────────────────────────────────────────────────────────

function showToast(message, kind = 'error') {
	const container = document.getElementById('toast-container');
	if (!container) return;
	const promo = document.getElementById('pwa-promo');
	const toast = document.createElement('div');
	// Class names spelled out so Tailwind's content scanner picks them up.
	const variant = kind === 'success' ? 'toast-success' : 'toast-error';
	toast.className = `toast ${variant}`;
	toast.textContent = message;
	container.appendChild(toast);
	// Toast and PWA promo share the header centre slot — fully remove the
	// promo from layout (display:none, not visibility:hidden) so the toast
	// takes its place instead of stacking under it. Restore on last toast end.
	const promoWasVisible = promo && promo.style.display === 'flex';
	if (promoWasVisible) promo.style.display = 'none';
	requestAnimationFrame(() => toast.classList.add('toast-visible'));
	setTimeout(() => {
		toast.classList.remove('toast-visible');
		setTimeout(() => {
			toast.remove();
			if (promoWasVisible && container.children.length === 0) {
				promo.style.display = 'flex';
			}
		}, 200);
	}, 4000);
}

// ── PWA promo ───────────────────────────────────────────────────────────────

const PWA_PROMO_DISMISSED = 'odio.pwa-promo.dismissed';

// The promo stays `hidden` by default to avoid a flash on load if dismissed.
// We override with inline `display: flex` rather than a class because
// Tailwind compiles `.hidden` after `.flex` in output.css, so a plain `flex`
// utility would lose the cascade.
function dismissPwaPromo() {
	localStorage.setItem(PWA_PROMO_DISMISSED, '1');
	const el = document.getElementById('pwa-promo');
	if (el) el.style.display = '';
}

function goToPwa() {
	let path = `/#/i/${window.location.hostname}`;
	if (window.location.port && window.location.port !== '8018') {
		path += `/${window.location.port}`;
	}
	dismissPwaPromo();
	window.open(`https://pwa.odio.love${path}`, '_blank', 'noopener,noreferrer');
}

document.addEventListener('DOMContentLoaded', () => {
	if (localStorage.getItem(PWA_PROMO_DISMISSED) === '1') return;
	const el = document.getElementById('pwa-promo');
	if (el) el.style.display = 'flex';
});

// HTMX errors: 4xx/5xx and network failures. Shuffle/loop also wire their own
// hx-on::*-error to revert the optimistic icon — that runs independently.
document.body.addEventListener('htmx:responseError', e => {
	const xhr = e.detail.xhr;
	const text = (xhr.responseText || '').trim();
	showToast(text || `${xhr.status} ${xhr.statusText}`);
});
document.body.addEventListener('htmx:sendError', () => {
	showToast('Network error');
});

// ── Transport buttons ───────────────────────────────────────────────────────

// Both icons live in the button as <span>; group-data-[playing=true]: variants
// flip which one is visible. JS flips data-playing, the title and the matching
// seeker so its interpolation stops/starts without waiting for the next HTMX
// poll. Position is also snapshotted to the visible slider value: data-position
// only refreshes on player.updated swaps (not heartbeat ticks), so without
// the snapshot the slider would briefly snap back to a stale base on pause.
// Revert is the same operation (idempotent).
function optimisticTogglePlayPause(button, playerName) {
	const next = button.dataset.playing !== 'true';
	button.dataset.playing = next;
	button.title = next ? 'Pause' : 'Play';
	const slider = document.querySelector(`.seek-slider[data-player="${CSS.escape(playerName)}"]`);
	if (slider) {
		const frozen = slider.value;
		const now = new Date().toISOString();
		const val = next ? 'true' : 'false';
		slider.dataset.position = frozen;
		slider.dataset.positionUpdatedAt = now;
		slider.dataset.playing = val;
		const span = slider.previousElementSibling;
		if (span) {
			span.dataset.position = frozen;
			span.dataset.positionUpdatedAt = now;
			span.dataset.playing = val;
		}
	}
}
const revertPlayPause = optimisticTogglePlayPause;

// Shuffle / loop are wired with HTMX (hx-post + hx-vals), but the button still
// flips its visual state in this onclick handler so the icon doesn't lag behind
// the SSE refresh. The previous state is stashed in a data attribute so the
// hx-on::*-error revert can restore it without re-querying the server.

function optimisticToggleShuffle(btn) {
	const prev = btn.dataset.shuffle === 'true';
	btn.dataset.shufflePrev = prev;
	const next = !prev;
	btn.dataset.shuffle = next;
	btn.classList.toggle('btn-active', next);
	btn.title = `Shuffle ${next ? 'on' : 'off'}`;
}

function revertShuffle(btn) {
	const prev = btn.dataset.shufflePrev === 'true';
	btn.dataset.shuffle = prev;
	btn.classList.toggle('btn-active', prev);
	btn.title = `Shuffle ${prev ? 'on' : 'off'}`;
}

// data-loop drives both the icon swap (via group-data-[loop=Track]: variants)
// and the active highlight (.btn-active when loop !== "None").
const LOOP_CYCLE = {None: 'Playlist', Playlist: 'Track', Track: 'None'};

function optimisticCycleLoop(btn) {
	const prev = btn.dataset.loop || 'None';
	const next = LOOP_CYCLE[prev] || 'Playlist';
	btn.dataset.loopPrev = prev;
	btn.dataset.loop = next;
	btn.classList.toggle('btn-active', next !== 'None');
	btn.title = `Repeat: ${next}`;
}

function revertLoop(btn) {
	const prev = btn.dataset.loopPrev || 'None';
	btn.dataset.loop = prev;
	btn.classList.toggle('btn-active', prev !== 'None');
	btn.title = `Repeat: ${prev}`;
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
	if (el.dataset.playing !== 'true' || !el.dataset.positionUpdatedAt) return base;
	const elapsed = Date.now() - new Date(el.dataset.positionUpdatedAt).getTime(); // ms
	return base + elapsed * 1000 * (parseFloat(el.dataset.rate) || 1.0); // → µs
}

// While dragging: update the position label without making any API call.
function onSeekerInput(slider) {
	slider.previousElementSibling.textContent = fmtMicros(parseInt(slider.value));
}

// On release: optimistically update data attributes so updatePositions
// interpolates from the new position instead of snapping back to the old
// cached one. The HTMX hx-post on the slider handles the server call.
function optimisticSeek(slider) {
	slider.dataset.position = parseInt(slider.value);
	slider.dataset.positionUpdatedAt = new Date().toISOString();
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

// ── Countdowns ───────────────────────────────────────────────────────────────

// Sections only re-render on SSE events, so a server-rendered deadline (e.g. the
// pairing timer) would freeze between events. Tick it down client-side from the
// absolute deadline in data-until. Queries the live DOM each time so HTMX
// section replacements need no re-initialisation (same approach as
// updatePositions). The element disappears on its own when the backend emits the
// state change that ends the countdown.
function updateCountdowns() {
	document.querySelectorAll('.countdown[data-until]').forEach(el => {
		const until = parseInt(el.dataset.until);
		const left = Math.max(0, Math.ceil((until - Date.now()) / 1000));
		el.textContent = `⏱ ${left}s`;
	});
}
document.addEventListener('DOMContentLoaded', () => {
	updateCountdowns();
	setInterval(updateCountdowns, 1000);
});
document.addEventListener('htmx:afterSwap', updateCountdowns);

// Open a service URL declared in the systemd config. Accepts ":port" /
// "/path" / "//host/..." / full URLs and resolves the missing parts against
// the page's current location so the link works from any host the dashboard
// is reachable from.
function openServiceUrl(u) {
	if (!u) return;
	if (u.startsWith(':')) {
		u = window.location.protocol + '//' + window.location.hostname + u;
	} else if (u.startsWith('/') && !u.startsWith('//')) {
		u = window.location.protocol + '//' + window.location.host + u;
	} else if (u.startsWith('//')) {
		u = window.location.protocol + u;
	}
	window.open(u, '_blank', 'noopener,noreferrer');
}

function closeSinkDropdown() {
	document.querySelectorAll('.sink-dropdown ul').forEach(ul => ul.classList.add('hidden'));
}

// Close sink dropdown when clicking outside
document.addEventListener('click', function(e) {
	if (!e.target.closest('.sink-dropdown')) {
		closeSinkDropdown();
	}
});

// Preserve <details> open/closed state across SSE innerHTML swaps.
// Capture state in sseBeforeMessage (fires before swap), restore in afterSwap.
var detailsState = {};
document.addEventListener('htmx:sseBeforeMessage', function(e) {
	document.querySelectorAll('details[id]').forEach(function(d) {
		detailsState[d.id] = d.open;
	});
});
document.addEventListener('htmx:afterSwap', function(e) {
	Object.keys(detailsState).forEach(function(id) {
		var el = document.getElementById(id);
		if (el && el.open !== detailsState[id]) {
			el.open = detailsState[id];
		}
	});
});

// Open the bluetooth devices dropdown when a scan starts. The dropdown may not
// exist yet (no devices), so we record the intent in detailsState — the same map
// the preservation above uses — so it opens as soon as the first device renders,
// and stays open as more stream in.
function openBluetoothDevices() {
	detailsState['bluetooth-devices'] = true;
	const el = document.getElementById('bluetooth-devices');
	if (el) el.open = true;
}

// Bluetooth connect is synchronous and can take seconds; the section also
// re-renders as a scan streams in, which would swap away the in-flight button.
// Track connecting addresses and reapply the "Connecting…" state after each swap
// (same idea as detailsState), clearing it once the row reports connected.
var btConnecting = new Set();

function btShowConnecting(btn) {
	btn.innerHTML = '<span class="spinner"></span>';
	btn.classList.add('pointer-events-none');
}

function btConnect(btn, address) {
	btConnecting.add(address);
	btShowConnecting(btn);
}

function btConnectFailed(btn, address) {
	btConnecting.delete(address);
	btn.textContent = 'Connect';
	btn.classList.remove('pointer-events-none');
}

document.addEventListener('htmx:afterSwap', function() {
	if (btConnecting.size === 0) return;
	document.querySelectorAll('#bluetooth-devices li[data-address]').forEach(function(li) {
		if (!btConnecting.has(li.dataset.address)) return;
		if (li.dataset.connected === 'true') {
			btConnecting.delete(li.dataset.address); // connected → row now shows Disconnect
			return;
		}
		const btn = li.querySelector('button');
		if (btn) btShowConnecting(btn);
	});
});

// Block SSE swaps on the audio section while the sink dropdown is open
document.addEventListener('htmx:sseBeforeMessage', function(e) {
	if (e.detail.elt && e.detail.elt.querySelector('.sink-dropdown ul:not(.hidden)')) {
		e.preventDefault();
	}
});

// Both icons are rendered in the button; toggling data-muted on the button
// flips which one is visible via Tailwind's group-data-[muted=true]: variants.
// Reverting an optimistic toggle is the same operation: flip data-muted again.
function optimisticToggleMute(button) {
	button.dataset.muted = button.dataset.muted === 'true' ? 'false' : 'true';
}
const revertMute = optimisticToggleMute;

// MPRIS has no backend mute property: we simulate it by sending volume=0 and
// remembering the previous volume in data-prev-volume so unmute can restore it.
// The sibling slider is moved optimistically so the change is immediate.
// data-target-volume is read by the button's hx-vals to build the POST body.
function optimisticToggleMuteMpris(button) {
	const slider = button.parentElement.querySelector('.volume-slider');
	if (!slider) return;
	const isMuted = button.dataset.muted === 'true';
	if (isMuted) {
		const prev = parseFloat(button.dataset.prevVolume) || 1;
		slider.value = Math.round(prev * 100);
		button.dataset.targetVolume = prev;
	} else {
		const current = parseInt(slider.value) / 100;
		if (current > 0) button.dataset.prevVolume = current;
		slider.value = 0;
		button.dataset.targetVolume = 0;
	}
	button.dataset.muted = isMuted ? 'false' : 'true';
}
const revertMuteMpris = optimisticToggleMuteMpris;
