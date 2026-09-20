import assert from "node:assert/strict"
import fs from "node:fs/promises"
import path from "node:path"
import { pathToFileURL } from "node:url"

const [pluginPath, scenario, root] = process.argv.slice(2)
const plugin = (await import(pathToFileURL(pluginPath))).default
const sessionDirectory = path.join(root, "session")
const workdir = path.join(sessionDirectory, "nested")
await fs.mkdir(workdir, { recursive: true })
if (scenario === "v1") {
  const hooks = await plugin({directory:workdir,worktree:workdir})
  await hooks["tool.execute.before"]({tool:"bash"})
  await hooks["tool.execute.after"]({tool:"bash"})
  assert.equal(await fs.readFile(path.join(workdir,"hook.log"),"utf8"),"beforeafter")
} else {
  assert.equal(plugin.id,"autopus.hooks")
  const callbacks = new Map()
  const disposed = []
  let lookups = 0
  const ctx = {
    location:{directory:"/wrong/plugin-instance-location"},
    session:{get:async ({sessionID})=>{
      assert.equal(sessionID,"session-owned")
      lookups++
      if (scenario === "missing-session") return {id:sessionID}
      return {id:sessionID,location:{directory:sessionDirectory}}
    }},
    tool:{hook:async (name,callback)=>{
      if (scenario === "setup-failure" && name === "execute.after") throw new Error("registration failed")
      assert.ok(["execute.before","execute.after"].includes(name))
      callbacks.set(name,callback)
      return {dispose:async()=>{
        disposed.push(name)
        if (scenario === "dispose-failure" && name === "execute.before") throw new Error("dispose failure")
      }}
    }}
  }
  if (scenario === "setup-failure") {
    await assert.rejects(()=>plugin.setup(ctx),/registration failed/)
    assert.deepEqual(disposed,["execute.before"])
  } else {
    const cleanup = await plugin.setup(ctx)
    const event = {tool:"shell",sessionID:"session-owned",id:"call-owned",input:{workdir:"nested"}}
    await callbacks.get("execute.before")({...event,tool:"read"})
    assert.equal(lookups,0)
    if (scenario === "normal" || scenario === "dispose-failure") {
      await callbacks.get("execute.before")(event)
      await callbacks.get("execute.after")({...event,status:"completed",result:{}})
      assert.equal(await fs.readFile(path.join(workdir,"hook.log"),"utf8"),"beforeafter")
      await callbacks.get("execute.before")({...event,id:"error-call"})
      await callbacks.get("execute.after")({...event,id:"error-call",status:"error",error:{message:"tool failed"}})
      assert.equal(await fs.readFile(path.join(workdir,"hook.log"),"utf8"),"beforeafterbeforeafter")
    } else {
      const started = Date.now()
      await assert.rejects(()=>callbacks.get("execute.before")(event),error=>{
        assert.ok(!String(error).includes("secret-credential"))
        return true
      })
      assert.ok(Date.now()-started<4000)
      await assert.rejects(fs.stat(path.join(workdir,"hook.log")))
      if (scenario === "timeout") {
        await new Promise(resolve=>setTimeout(resolve,1300))
        await assert.rejects(fs.stat(path.join(workdir,"escaped.log")))
      }
    }
    if (scenario === "dispose-failure") await assert.rejects(cleanup,/dispose failure/)
    else await cleanup()
    assert.deepEqual(disposed.sort(),["execute.after","execute.before"])
  }
}
console.log("CONTRACT_PASS")
