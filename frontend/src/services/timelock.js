import { Buffer } from 'buffer'
import { defaultChainInfo, mainnetClient, roundAt, roundTime, timelockEncrypt } from 'tlock-js'

// quicknet publishes a round every 3 seconds. Encrypting to the next round after the
// deadline means the picks cannot be opened before entries close.
let client
const chain = () => {
  client ??= mainnetClient()
  return client
}

export async function lockUntil (bytes, deadline) {
  const round = roundAt(deadline * 1000, defaultChainInfo) + 1
  const opens = Math.floor(roundTime(defaultChainInfo, round) / 1000)
  if (opens < deadline) throw new Error('The time lock opens before entries close')
  return timelockEncrypt(round, Buffer.from(bytes), chain())
}
