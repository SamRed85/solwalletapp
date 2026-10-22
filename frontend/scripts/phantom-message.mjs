import { compileMessage, placeCouponIx } from '../src/services/program.js'
import { phantomTransaction } from '../src/services/solana.js'

const player = '955GUT5WguA7BJr1J9JVzRnSkqr1uszy2yHGBCz6Adun'
const admin = '4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH'
const instruction = placeCouponIx({
  player,
  admin,
  nonce: 1,
  index: 0,
  commitment: new Uint8Array(32).fill(7),
  combinations: 1
})
const { message } = compileMessage(player, [instruction], player)
const wrapped = phantomTransaction(message)
if (wrapped.length !== message.length + 65) throw new Error('Phantom payload is the wrong size')
if (wrapped[0] !== 1) throw new Error('Phantom payload needs one signature slot')
for (let index = 0; index < message.length; index++) {
  if (wrapped[65 + index] !== message[index]) throw new Error('Phantom payload does not wrap the program message')
}
console.log('phantom payload wraps the program message')
