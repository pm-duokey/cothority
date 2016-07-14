package jvss

import (
	"fmt"

	"github.com/dedis/cothority/log"
	"github.com/dedis/cothority/sda"
	"github.com/dedis/crypto/poly"
)

// SecInitMsg are used to initialise new shared secrets both long- and
// short-term.
type SecInitMsg struct {
	Src  int
	SID  SID
	Deal []byte
}

// SecConfMsg are used to confirm to other peers that we have finished setting
// up the shared secret.
type SecConfMsg struct {
	Src int
	SID SID
}

// SigReqMsg are used to send signing requests.
type SigReqMsg struct {
	Src int
	SID SID
	Msg []byte
}

// SigRespMsg are used to reply to signing requests.
type SigRespMsg struct {
	Src int
	SID SID
	Sig *poly.SchnorrPartialSig
}

// WSecInitMsg is a SDA-wrapper around SecInitMsg.
type WSecInitMsg struct {
	*sda.TreeNode
	SecInitMsg
}

// WSecConfMsg is a SDA-wrapper around SecConfMsg.
type WSecConfMsg struct {
	*sda.TreeNode
	SecConfMsg
}

// WSigReqMsg is a SDA-wrapper around SigReqMsg.
type WSigReqMsg struct {
	*sda.TreeNode
	SigReqMsg
}

// WSigRespMsg is a SDA-wrapper around SigRespMsg.
type WSigRespMsg struct {
	*sda.TreeNode
	SigRespMsg
}

func (jv *JVSS) handleSecInit(m WSecInitMsg) error {
	msg := m.SecInitMsg

	log.Lvl2(jv.Name(), jv.Index(), "Received SecInit from", m.TreeNode.Name())

	// Initialise shared secret
	if err := jv.initSecret(msg.SID); err != nil {
		return err
	}

	// Unmarshal received deal
	deal := new(poly.Deal).UnmarshalInit(jv.info.T, jv.info.R, jv.info.N, jv.keyPair.Suite)
	if err := deal.UnmarshalBinary(msg.Deal); err != nil {
		return err
	}

	// Buffer received deal for later
	secret, err := jv.secrets.secret(msg.SID)
	if err != nil {
		return err
	}
	secret.deals[msg.Src] = deal

	// Finalise shared secret
	if err := jv.finaliseSecret(msg.SID); err != nil {
		return err
	}
	log.Lvl2("Finalised secret", jv.Name(), msg.SID)
	return nil
}

func (jv *JVSS) handleSecConf(m WSecConfMsg) error {
	msg := m.SecConfMsg
	secret, err := jv.secrets.secret(msg.SID)
	if err != nil {
		return err
	}

	secret.nConfirmsMtx.Lock()
	defer secret.nConfirmsMtx.Unlock()
	secret.numConfs++

	// Check if we are the initiator node and have enough confirmations to proceed
	if (secret.numConfs == len(jv.List())) && msg.SID.IsLTSS() && jv.sidStore.exists(msg.SID) {
		log.Lvl4("Writing to longTermSecDone")
		jv.longTermSecDone <- true
		secret.numConfs = 0
	} else if (secret.numConfs == len(jv.List())) && msg.SID.IsSTSS() && jv.sidStore.exists(msg.SID) {
		log.LLvl4("Writing to shortTermSecDone")
		jv.shortTermSecDone <- true
		secret.numConfs = 0
	} else {
		log.Lvl2(fmt.Sprintf("Node %d: %s confirmations %d/%d", jv.Index(), msg.SID, secret.numConfs, len(jv.List())))
	}

	return nil
}

func (jv *JVSS) handleSigReq(m WSigReqMsg) error {
	msg := m.SigReqMsg

	// Create partial signature
	ps, err := jv.sigPartial(msg.SID, msg.Msg)
	if err != nil {
		return err
	}

	// Send it back to initiator
	resp := &SigRespMsg{
		Src: jv.Index(),
		SID: msg.SID,
		Sig: ps,
	}

	if err := jv.SendTo(jv.List()[msg.Src], resp); err != nil {
		return err
	}

	// Cleanup short-term shared secret
	jv.secrets.remove(msg.SID)

	return nil
}

func (jv *JVSS) handleSigResp(m WSigRespMsg) error {
	msg := m.SigRespMsg

	// Collect partial signatures
	secret, err := jv.secrets.secret(msg.SID)
	if err != nil {
		return err
	}

	secret.sigs[msg.Src] = msg.Sig

	log.Lvl2(fmt.Sprintf("Node %d: %s signatures %d/%d", jv.Index(), msg.SID, len(secret.sigs), len(jv.List())))

	// Create Schnorr signature once we received enough partial signatures
	if jv.info.T == len(secret.sigs) {

		for _, sig := range secret.sigs {
			if err := jv.schnorr.AddPartialSig(sig); err != nil {
				return err
			}
		}

		sig, err := jv.schnorr.Sig()
		if err != nil {
			return err
		}
		jv.sigChan <- sig

		// Cleanup short-term shared secret
		jv.secrets.remove(msg.SID)
	}

	return nil
}
