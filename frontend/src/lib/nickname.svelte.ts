import { browser } from '$app/env';

const KEY = 'nickname';

// localStorage can throw (private mode, blocked site data); the nickname then
// just isn't remembered across reloads.
function load(): string {
	if (!browser) return '';
	try {
		return localStorage.getItem(KEY) ?? '';
	} catch {
		return '';
	}
}

class NicknameState {
	#name = $state(load());
	get name() {
		return this.#name;
	}
	set name(val: string) {
		this.#name = val;
		try {
			localStorage.setItem(KEY, val);
		} catch {
			/* storage unavailable */
		}
	}
	get hasNickname() {
		return this.#name.length > 0;
	}
}

/** This viewer's self-chosen display name, sent with every API call. */
export const nickname = new NicknameState();
