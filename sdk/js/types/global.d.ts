// SPDX-License-Identifier: MIT
// 호스트가 프레임에 주는 SDK 런타임(_maru/sdk.js)의 전역 이름. 프레임 코드에서 import 없이 maru를 쓸 수 있게 한다.
import type { MaruNamespace } from './api'

declare global {
  /** 호스트가 주는 SDK 런타임(_maru/sdk.js). */
  const maru: MaruNamespace
}
