// Test script to check Solana RPC endpoints
const { Connection } = require('@solana/web3.js')

const RPC_URLS = [
  'https://api.devnet.solana.com',
  'https://rpc.ankr.com/solana_devnet',
  'https://solana-devnet.g.alchemy.com/v2/demo'
]

async function testRPCConnection(url) {
  try {
    console.log(`Testing: ${url}`)
    const connection = new Connection(url, 'confirmed')
    const version = await connection.getVersion()
    console.log(`✅ SUCCESS: ${url}`)
    console.log(`   Version: ${version['solana-core']}`)
    return true
  } catch (error) {
    console.log(`❌ FAILED: ${url}`)
    console.log(`   Error: ${error.message}`)
    return false
  }
}

async function testAllRPCs() {
  console.log('Testing Solana RPC endpoints...\n')
  
  let workingEndpoints = []
  
  for (const url of RPC_URLS) {
    const success = await testRPCConnection(url)
    if (success) {
      workingEndpoints.push(url)
    }
    console.log('') // Empty line for readability
  }
  
  console.log('Summary:')
  if (workingEndpoints.length > 0) {
    console.log(`✅ ${workingEndpoints.length} working endpoint(s):`)
    workingEndpoints.forEach(url => console.log(`   - ${url}`))
  } else {
    console.log('❌ No working endpoints found')
    console.log('   This might be a network connectivity issue')
  }
}

testAllRPCs().catch(console.error)
