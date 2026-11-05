// 애드온 프레임 브리지 요청의 실제 처리. 대시보드 API를 이 세션으로 부르고, 확인 대화상자·알림·워크벤치 열기는
// 호스트 화면에 패키지 이름을 붙여 보인다. 판정(받을지, 파라미터)은 bridge.ts가 먼저 한다.
import type { BridgeError, RequestMethod, RequestParams } from '@maru-sdk/protocol'
import { BRIDGE_CODES, interpolate } from '@maru-sdk/protocol'
import i18n from 'i18next'
import {
  ApiError,
  addonPackageRPC,
  fetchAddonPackageData,
  fetchAddonStorage,
  runAddonPackageAction,
  saveAddonStorage,
} from '../api/client'
import type { AddonPackageView } from '../api/types.gen'
import { notify } from '../notifications/notify'
import { requestStepUp } from '../security/stepUpDialog'
import { confirmDialog } from '../shell/dialogPresets'
import type { DialogScope } from '../shell/dialogs'
import { openWorkbenchRoute } from '../shell/launchApp'
import { localizedText } from './catalog'
import { plainText } from './bridge'

export interface RequestContext {
  pkg: AddonPackageView
  // scope는 확인 대화상자를 그릴 곳이다(창 보기는 그 창, 위젯은 없음).
  scope: DialogScope | null
  language: string
  messages: Record<string, string>
}

// BridgeFailure는 요청이 실패했다는 뜻이다. 프레임에는 error만 넘긴다.
export class BridgeFailure extends Error {
  readonly error: BridgeError
  constructor(error: BridgeError) {
    super(error.message)
    this.error = error
  }
}

function packageName(ctx: RequestContext): string {
  return plainText(localizedText(ctx.pkg.name, ctx.language), 40)
}

// toBridgeError는 실패를 프레임에 넘길 값으로 바꾼다. 대시보드 API 오류는 상태와 코드를, 그 밖은 메시지만 준다.
// 패키지 백엔드의 오류(addon_error)는 백엔드가 준 문구를 메시지로 준다.
export function toBridgeError(err: unknown): BridgeError {
  if (err instanceof BridgeFailure) return err.error
  if (err instanceof ApiError) {
    return { status: err.status, code: err.code, addonCode: err.addonCode, message: err.addonMessage ?? err.message }
  }
  return { status: 0, message: err instanceof Error ? err.message : String(err) }
}

// runAction은 조작을 실행한다. manifest가 확인을 요구하면 호스트 대화상자로 먼저 묻는다(프레임이 건너뛸 수 없다).
// 쓰기를 선언한 패키지는 백엔드가 단계 확인을 요구하면(security_required) 확인한 뒤 한 번 다시 보낸다.
async function runAction(ctx: RequestContext, p: RequestParams['action']): Promise<unknown> {
  const decl = ctx.pkg.actions.find((a) => a.name === p.name)
  if (!decl) throw new BridgeFailure({ status: 0, code: BRIDGE_CODES.notAllowed, message: `unknown action ${p.name}` })
  if (decl.confirm) {
    if (!ctx.scope) throw new BridgeFailure({ status: 0, code: BRIDGE_CODES.notAllowed, message: 'no dialog scope' })
    const ok = await confirmDialog(ctx.scope, {
      title: i18n.t('addonPackages.confirmTitle', { name: packageName(ctx) }),
      message: plainText(localizedText(decl.confirmText, ctx.language) || localizedText(decl.label, ctx.language) || p.name, 300),
      confirmLabel: plainText(localizedText(decl.label, ctx.language), 40) || undefined,
      danger: true,
    })
    if (!ok) throw new BridgeFailure({ status: 0, code: BRIDGE_CODES.cancelled, message: 'cancelled' })
  }
  try {
    return await runAddonPackageAction(ctx.pkg.id, p.name, p.target ?? '', p.input, { lang: ctx.language })
  } catch (err) {
    if (err instanceof ApiError && err.code === 'security_required' && ctx.scope) {
      const st = await requestStepUp(ctx.scope, { level: err.required })
      if (st) return runAddonPackageAction(ctx.pkg.id, p.name, p.target ?? '', p.input, { lang: ctx.language })
    }
    throw err
  }
}

// handleRequest는 받은 요청 하나를 처리하고 프레임에 줄 결과를 돌려준다. 실패하면 던진다(toBridgeError로 바꾼다).
export async function handleRequest<M extends RequestMethod>(
  method: M,
  params: RequestParams[M],
  ctx: RequestContext,
): Promise<unknown> {
  const id = ctx.pkg.id
  switch (method) {
    case 'data':
      return fetchAddonPackageData(id, (params as RequestParams['data']).query ?? {}, { lang: ctx.language })
    case 'rpc': {
      const p = params as RequestParams['rpc']
      return addonPackageRPC(id, p.service, p.method, p.params)
    }
    case 'action':
      await runAction(ctx, params as RequestParams['action'])
      return null
    case 'query':
      return runAction(ctx, params as RequestParams['query'])
    case 'storage.get':
      return fetchAddonStorage(id)
    case 'storage.set':
      await saveAddonStorage(id, (params as RequestParams['storage.set']).data)
      return null
    case 'openWorkbench':
      openWorkbenchRoute((params as RequestParams['openWorkbench']).route)
      return null
    case 'confirm': {
      const p = params as RequestParams['confirm']
      if (!ctx.scope) throw new BridgeFailure({ status: 0, code: BRIDGE_CODES.notAllowed, message: 'no dialog scope' })
      const title = p.title ? plainText(p.title, 80) : ''
      return confirmDialog(ctx.scope, {
        title: i18n.t('addonPackages.confirmTitle', { name: packageName(ctx) }),
        message: title ? `${title}: ${plainText(p.message, 600)}` : plainText(p.message, 600),
        confirmLabel: p.confirmLabel ? plainText(p.confirmLabel, 40) : undefined,
        danger: p.danger,
      })
    }
    case 'notify': {
      const p = params as RequestParams['notify']
      const text = plainText(interpolate(ctx.messages[p.key] ?? p.key, p.params), 300)
      notify({
        code: 'addonMessage',
        level: p.level,
        params: { name: packageName(ctx), text },
        action: ctx.pkg.views.window ? { kind: 'app', app: id } : undefined,
      })
      return null
    }
  }
  throw new BridgeFailure({ status: 0, code: BRIDGE_CODES.invalid, message: `unknown method ${method}` })
}
