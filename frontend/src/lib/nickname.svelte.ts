import { browser } from '$app/env';
import { getContext, setContext } from 'svelte';

class NicknameState {
	#name = $state(browser ? (localStorage.getItem('nickname') ?? '') : '');
	get name() {
		return this.#name;
	}
	set name(val: string) {
		this.#name = val;
		if (browser) localStorage.setItem('nickname', val);
	}
	get hasNickname() {
		return this.#name.length > 0;
	}
}

export const nickname = new NicknameState();

const NICKNAME_KEY = Symbol('nickname');
export function setNicknameContext() {
	return setContext(NICKNAME_KEY, nickname);
}
export function getNicknameContext() {
	return getContext<NicknameState>(NICKNAME_KEY);
}
