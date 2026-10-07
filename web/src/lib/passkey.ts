import { api, unwrap } from "@/lib/api"

type JSONOptions = { publicKey: Record<string, unknown> }

export function passkeysSupported() {
  return typeof window.PublicKeyCredential !== "undefined" && "parseRequestOptionsFromJSON" in PublicKeyCredential
}

export async function signInWithPasskey() {
  const ceremony = await unwrap(api.POST("/v1/auth/passkey/options"))
  const options = (ceremony.options as JSONOptions).publicKey
  const credential = (await navigator.credentials.get({
    publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(
      options as unknown as PublicKeyCredentialRequestOptionsJSON,
    ),
  })) as PublicKeyCredential | null
  if (!credential) throw new Error("cancelled")
  return unwrap(
    api.POST("/v1/auth/passkey", {
      body: { ceremony_id: ceremony.ceremony_id, credential: credential.toJSON() as unknown as Record<string, unknown> },
    }),
  )
}

export async function registerPasskey(name: string) {
  const ceremony = await unwrap(api.POST("/v1/me/passkeys/options"))
  const options = (ceremony.options as JSONOptions).publicKey
  const credential = (await navigator.credentials.create({
    publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(
      options as unknown as PublicKeyCredentialCreationOptionsJSON,
    ),
  })) as PublicKeyCredential | null
  if (!credential) throw new Error("cancelled")
  return unwrap(
    api.POST("/v1/me/passkeys", {
      body: {
        ceremony_id: ceremony.ceremony_id,
        name: name || undefined,
        credential: credential.toJSON() as unknown as Record<string, unknown>,
      },
    }),
  )
}
